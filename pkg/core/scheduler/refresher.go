package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/audit"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/config"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/notify"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/store"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/vault"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers/acck"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers/akile"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers/lazycat"
)

type loginFailureRecord struct {
	CredHash string
	FailedAt time.Time
}

var (
	cooldownMu       sync.Mutex
	autoLoginCoolMap = make(map[string]loginFailureRecord)
)

const invalidCredCooldown = 60 * time.Minute

func credFingerprint(username, password string) string {
	sum := sha256.Sum256([]byte(username + "\x00" + password))
	return hex.EncodeToString(sum[:8])
}

func shouldSkipAutoLogin(providerID, username, password string) bool {
	cooldownMu.Lock()
	defer cooldownMu.Unlock()
	rec, ok := autoLoginCoolMap[providerID]
	if !ok {
		return false
	}
	if rec.CredHash != credFingerprint(username, password) {
		delete(autoLoginCoolMap, providerID)
		return false
	}
	return time.Since(rec.FailedAt) < invalidCredCooldown
}

func recordAutoLoginFailure(providerID, username, password string) {
	cooldownMu.Lock()
	autoLoginCoolMap[providerID] = loginFailureRecord{
		CredHash: credFingerprint(username, password),
		FailedAt: time.Now(),
	}
	cooldownMu.Unlock()
}

func clearAutoLoginFailure(providerID string) {
	cooldownMu.Lock()
	delete(autoLoginCoolMap, providerID)
	cooldownMu.Unlock()
}

func parseJWTExpiration(tok string) (time.Time, bool) {
	tok = strings.Trim(strings.TrimSpace(tok), "\"'")
	if strings.HasPrefix(strings.ToLower(tok), "bearer ") {
		tok = strings.TrimSpace(tok[7:])
	}
	parts := strings.Split(tok, ".")
	if len(parts) < 2 {
		return time.Time{}, false
	}

	payload := parts[1]
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}

	data, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		data, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return time.Time{}, false
		}
	}

	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(data, &claims); err != nil || claims.Exp <= 0 {
		return time.Time{}, false
	}

	return time.Unix(claims.Exp, 0), true
}

// StartTokenLifecycleRefresher 启动后台全自动定时登录与 Token 续期守护协程
func StartTokenLifecycleRefresher(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	// 启动时立即执行一次巡检
	checkAndRefreshTokens(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			checkAndRefreshTokens(ctx)
		}
	}
}

func generateTOTPCode(secret string) string {
	if strings.TrimSpace(secret) == "" {
		return ""
	}
	code, err := vault.GenerateTOTP(secret)
	if err != nil {
		log.Printf("[TokenRefresher] 生成 2FA 动态码失败: %v", err)
		return ""
	}
	return code
}

func checkAndRefreshTokens(ctx context.Context) {
	st := config.LoadSavedTokens()
	creds := config.LoadSavedCredentials()

	// 1. Akile Token 自动续期与免密重登
	akileTok := st.AkileToken
	if akileTok == "" {
		akileTok = config.GetProviderToken("akile")
	}

	if akileTok != "" {
		expTime, hasExp := parseJWTExpiration(akileTok)
		if hasExp {
			timeUntilExp := time.Until(expTime)

			p, ok := providers.Get("akile")
			if ok {
				if akProvider, isAk := p.(*akile.AkileProvider); isAk {
					// 场景 A: Token 仍在有效期内，但剩余时间少于 8 小时 -> 调用 refreshToken 换取新 24h Token
					if timeUntilExp > 0 && timeUntilExp < 8*time.Hour {
						log.Printf("[TokenRefresher] Akile Token 即将到期 (剩余 %.1f 小时)，正在自动续期...", timeUntilExp.Hours())
						subCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
						newTok, err := akProvider.RefreshToken(subCtx)
						cancel()
						if err == nil && newTok != "" {
							_ = config.SaveTokens(newTok, "", "", "")
							akProvider.SetToken(newTok)
							clearAutoLoginFailure("akile")
							audit.Record("daemon", "token.auto_refresh", "akile", nil, "SUCCESS", "Akile Token 定时自动续期成功，有效期已重置为 24 小时", 0)
							log.Printf("[TokenRefresher] Akile Token 自动续期成功！")
						} else if err != nil {
							log.Printf("[TokenRefresher] Akile Token 自动续期失败: %v", err)
						}
					}

					// 场景 B: Token 已过期 (timeUntilExp <= 0)，若配置了保存凭据且开启自动重登 -> 自动登录 (含 2FA TOTP)
					if timeUntilExp <= 0 && creds.Akile != nil && creds.Akile.Username != "" && creds.Akile.Password != "" && creds.Akile.AutoRefresh {
						if !shouldSkipAutoLogin("akile", creds.Akile.Username, creds.Akile.Password) {
							log.Printf("[TokenRefresher] Akile Token 已过期，尝试使用已保存凭据免密重新登录...")
							gaCode := generateTOTPCode(creds.Akile.TOTPSecret)
							subCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
							newTok, need2FA, _, err := akProvider.Login(subCtx, creds.Akile.Username, creds.Akile.Password, gaCode)
							cancel()
							if err == nil && !need2FA && newTok != "" {
								_ = config.SaveTokens(newTok, "", "", "")
								akProvider.SetToken(newTok)
								clearAutoLoginFailure("akile")
								audit.Record("daemon", "token.auto_relogin", "akile", nil, "SUCCESS", "Akile Token 已过期，后台使用已保存的账号密码(及2FA)免密自动重新登录成功", 0)
								log.Printf("[TokenRefresher] Akile 后台免密重新登录成功，新 Token 已生效！")
							} else if err != nil {
								recordAutoLoginFailure("akile", creds.Akile.Username, creds.Akile.Password)
								log.Printf("[TokenRefresher] Akile 后台免密重新登录失败: %v", err)
							}
						}
					}
				}
			}
		}
	} else if creds.Akile != nil && creds.Akile.Username != "" && creds.Akile.Password != "" && creds.Akile.AutoRefresh {
		// 场景 C: 从未配置 Token，但保存了账号密码 -> 自动初次登录
		if !shouldSkipAutoLogin("akile", creds.Akile.Username, creds.Akile.Password) {
			p, ok := providers.Get("akile")
			if ok {
				if akProvider, isAk := p.(*akile.AkileProvider); isAk {
					gaCode := generateTOTPCode(creds.Akile.TOTPSecret)
					subCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
					newTok, need2FA, _, err := akProvider.Login(subCtx, creds.Akile.Username, creds.Akile.Password, gaCode)
					cancel()
					if err == nil && !need2FA && newTok != "" {
						_ = config.SaveTokens(newTok, "", "", "")
						akProvider.SetToken(newTok)
						clearAutoLoginFailure("akile")
						audit.Record("daemon", "token.auto_login", "akile", nil, "SUCCESS", "后台使用已保存账号密码自动登录 Akile 成功并注入 Token", 0)
					} else if err != nil {
						recordAutoLoginFailure("akile", creds.Akile.Username, creds.Akile.Password)
					}
				}
			}
		}
	}

	// 2. 懒猫云 (LazyCat) 主动提前续签 (< 12 小时) 与到期免密重登 (含 2FA TOTP)
	lzTok := st.LazyCatToken
	if lzTok == "" {
		lzTok = config.GetProviderToken("lazycat")
	}

	if lzTok != "" {
		expTime, hasExp := parseJWTExpiration(lzTok)
		if hasExp && time.Until(expTime) < 12*time.Hour && creds.LazyCat != nil && creds.LazyCat.Username != "" && creds.LazyCat.Password != "" && creds.LazyCat.AutoRefresh {
			if !shouldSkipAutoLogin("lazycat", creds.LazyCat.Username, creds.LazyCat.Password) {
				p, ok := providers.Get("lazycat")
				if ok {
					if lzProvider, isLz := p.(*lazycat.LazyCatProvider); isLz {
						hoursLeft := time.Until(expTime).Hours()
						log.Printf("[TokenRefresher] 懒猫云 Token 临期/过期 (剩余 %.1f 小时)，正在自动免密续签...", hoursLeft)
						totpCode := generateTOTPCode(creds.LazyCat.TOTPSecret)
						subCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
						newTok, need2FA, _, err := lzProvider.Login(subCtx, creds.LazyCat.Username, creds.LazyCat.Password, totpCode)
						cancel()
						if err == nil && !need2FA && newTok != "" {
							_ = config.SaveTokens("", "", newTok, "")
							lzProvider.SetToken(newTok)
							clearAutoLoginFailure("lazycat")
							audit.Record("daemon", "token.auto_relogin", "lazycat", nil, "SUCCESS", "懒猫云 Token 临期/过期，后台自动免密续签成功", 0)
							log.Printf("[TokenRefresher] 懒猫云自动免密续签成功，新 Token 已生效！")
						} else if err != nil {
							recordAutoLoginFailure("lazycat", creds.LazyCat.Username, creds.LazyCat.Password)
							log.Printf("[TokenRefresher] 懒猫云自动免密续签失败: %v", err)
						} else if need2FA {
							log.Printf("[TokenRefresher] 懒猫云自动续签需要 2FA 动态码，但未配置有效 TOTP 密钥")
						}
					}
				}
			}
		}
	} else if creds.LazyCat != nil && creds.LazyCat.Username != "" && creds.LazyCat.Password != "" && creds.LazyCat.AutoRefresh {
		if !shouldSkipAutoLogin("lazycat", creds.LazyCat.Username, creds.LazyCat.Password) {
			p, ok := providers.Get("lazycat")
			if ok {
				if lzProvider, isLz := p.(*lazycat.LazyCatProvider); isLz {
					totpCode := generateTOTPCode(creds.LazyCat.TOTPSecret)
					subCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
					newTok, need2FA, _, err := lzProvider.Login(subCtx, creds.LazyCat.Username, creds.LazyCat.Password, totpCode)
					cancel()
					if err == nil && !need2FA && newTok != "" {
						_ = config.SaveTokens("", "", newTok, "")
						lzProvider.SetToken(newTok)
						clearAutoLoginFailure("lazycat")
						audit.Record("daemon", "token.auto_login", "lazycat", nil, "SUCCESS", "后台使用已保存账号密码自动登录懒猫云成功并注入 Token", 0)
					} else if err != nil {
						recordAutoLoginFailure("lazycat", creds.LazyCat.Username, creds.LazyCat.Password)
					}
				}
			}
		}
	}

	// 3. ACCK 主动提前续签 (< 24 小时) 与过期/缺失免密重登 (Altcha PoW + 2FA TOTP)
	acckTok := st.ACCKToken
	if acckTok == "" {
		acckTok = config.GetProviderToken("acck")
	}

	if creds.ACCK != nil && creds.ACCK.Username != "" && creds.ACCK.Password != "" && creds.ACCK.AutoRefresh {
		needsAcckRefresh := false
		reason := ""
		if acckTok == "" {
			needsAcckRefresh = true
			reason = "未配置或 Token 为空"
		} else if expTime, hasExp := parseJWTExpiration(acckTok); hasExp && time.Until(expTime) < 24*time.Hour {
			needsAcckRefresh = true
			hoursLeft := time.Until(expTime).Hours()
			if hoursLeft <= 0 {
				reason = fmt.Sprintf("Token 已过期 (%.1f 小时前)", -hoursLeft)
			} else {
				reason = fmt.Sprintf("Token 临期 (剩余 %.1f 小时)", hoursLeft)
			}
		}

		if needsAcckRefresh && !shouldSkipAutoLogin("acck", creds.ACCK.Username, creds.ACCK.Password) {
			log.Printf("[TokenRefresher] ACCK %s，正在执行后台自动免密登录...", reason)
			subCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
			ok, err := store.ForceAutoLoginACCK(subCtx)
			cancel()
			if ok && err == nil {
				clearAutoLoginFailure("acck")
				audit.Record("daemon", "token.auto_relogin", "acck", map[string]interface{}{
					"username": config.MaskUsername(creds.ACCK.Username),
					"reason":   reason,
				}, "SUCCESS", "ACCK 后台自动解算 Altcha 并免密登录成功，全新 7 天 Token 已生效", 0)
				log.Printf("[TokenRefresher] ACCK 自动免密登录成功，新 Token 已生效！")
			} else if err != nil {
				latest := config.LoadSavedCredentials()
				u, pw := creds.ACCK.Username, creds.ACCK.Password
				if latest.ACCK != nil && latest.ACCK.Password != "" {
					u, pw = latest.ACCK.Username, latest.ACCK.Password
				}
				if errors.Is(err, acck.ErrInvalidCredentials) {
					recordAutoLoginFailure("acck", u, pw)
					_ = notify.SendTelegram(fmt.Sprintf("⚠️ ACCK 后台自动登录失败（账号密码被拒绝，已进入 60 分钟保护冷却）：%v", err))
				}
				audit.Record("daemon", "token.auto_relogin", "acck", map[string]interface{}{
					"username": config.MaskUsername(u),
					"reason":   reason,
				}, "FAILED", fmt.Sprintf("ACCK 自动登录失败: %v", err), 0)
				log.Printf("[TokenRefresher] ACCK 自动登录失败: %v", err)
			}
		}
	}
}

package scheduler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/audit"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/config"
	"github.com/ashanzzz/personal-vps-monitor/pkg/core/vault"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers/akile"
	"github.com/ashanzzz/personal-vps-monitor/pkg/providers/lazycat"
)

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
		akileTok = config.Global.AkileToken
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
						if newTok, err := akProvider.RefreshToken(ctx); err == nil && newTok != "" {
							_ = config.SaveTokens(newTok, "", "", "")
							config.Global.AkileToken = newTok
							akProvider.SetToken(newTok)
							audit.Record("daemon", "token.auto_refresh", "akile", nil, "SUCCESS", "Akile Token 定时自动续期成功，有效期已重置为 24 小时", 0)
							log.Printf("[TokenRefresher] Akile Token 自动续期成功！")
						} else if err != nil {
							log.Printf("[TokenRefresher] Akile Token 自动续期失败: %v", err)
						}
					}

					// 场景 B: Token 已过期 (timeUntilExp <= 0)，若配置了保存凭据且开启自动重登 -> 自动登录 (含 2FA TOTP)
					if timeUntilExp <= 0 && creds.Akile != nil && creds.Akile.Username != "" && creds.Akile.Password != "" && creds.Akile.AutoRefresh {
						log.Printf("[TokenRefresher] Akile Token 已过期，尝试使用已保存凭据免密重新登录...")
						gaCode := generateTOTPCode(creds.Akile.TOTPSecret)
						if newTok, need2FA, _, err := akProvider.Login(ctx, creds.Akile.Username, creds.Akile.Password, gaCode); err == nil && !need2FA && newTok != "" {
							_ = config.SaveTokens(newTok, "", "", "")
							config.Global.AkileToken = newTok
							akProvider.SetToken(newTok)
							audit.Record("daemon", "token.auto_relogin", "akile", nil, "SUCCESS", "Akile Token 已过期，后台使用已保存的账号密码(及2FA)免密自动重新登录成功", 0)
							log.Printf("[TokenRefresher] Akile 后台免密重新登录成功，新 Token 已生效！")
						} else if err != nil {
							log.Printf("[TokenRefresher] Akile 后台免密重新登录失败: %v", err)
						}
					}
				}
			}
		}
	} else if creds.Akile != nil && creds.Akile.Username != "" && creds.Akile.Password != "" && creds.Akile.AutoRefresh {
		// 场景 C: 从未配置 Token，但保存了账号密码 -> 自动初次登录
		p, ok := providers.Get("akile")
		if ok {
			if akProvider, isAk := p.(*akile.AkileProvider); isAk {
				gaCode := generateTOTPCode(creds.Akile.TOTPSecret)
				if newTok, need2FA, _, err := akProvider.Login(ctx, creds.Akile.Username, creds.Akile.Password, gaCode); err == nil && !need2FA && newTok != "" {
					_ = config.SaveTokens(newTok, "", "", "")
					config.Global.AkileToken = newTok
					akProvider.SetToken(newTok)
					audit.Record("daemon", "token.auto_login", "akile", nil, "SUCCESS", "后台使用已保存账号密码自动登录 Akile 成功并注入 Token", 0)
				}
			}
		}
	}

	// 2. 懒猫云 (LazyCat) 主动提前续签 (< 12 小时) 与到期免密重登 (含 2FA TOTP)
	lzTok := st.LazyCatToken
	if lzTok == "" {
		lzTok = config.Global.LazyCatToken
	}

	if lzTok != "" {
		expTime, hasExp := parseJWTExpiration(lzTok)
		if hasExp && time.Until(expTime) < 12*time.Hour && creds.LazyCat != nil && creds.LazyCat.Username != "" && creds.LazyCat.Password != "" && creds.LazyCat.AutoRefresh {
			p, ok := providers.Get("lazycat")
			if ok {
				if lzProvider, isLz := p.(*lazycat.LazyCatProvider); isLz {
					hoursLeft := time.Until(expTime).Hours()
					log.Printf("[TokenRefresher] 懒猫云 Token 临期/过期 (剩余 %.1f 小时)，正在自动免密续签...", hoursLeft)
					totpCode := generateTOTPCode(creds.LazyCat.TOTPSecret)
					if newTok, need2FA, _, err := lzProvider.Login(ctx, creds.LazyCat.Username, creds.LazyCat.Password, totpCode); err == nil && !need2FA && newTok != "" {
						_ = config.SaveTokens("", "", newTok, "")
						config.Global.LazyCatToken = newTok
						lzProvider.SetToken(newTok)
						audit.Record("daemon", "token.auto_relogin", "lazycat", nil, "SUCCESS", "懒猫云 Token 临期/过期，后台自动免密续签成功", 0)
						log.Printf("[TokenRefresher] 懒猫云自动免密续签成功，新 Token 已生效！")
					} else if err != nil {
						log.Printf("[TokenRefresher] 懒猫云自动免密续签失败: %v", err)
					} else if need2FA {
						log.Printf("[TokenRefresher] 懒猫云自动续签需要 2FA 动态码，但未配置有效 TOTP 密钥")
					}
				}
			}
		}
	} else if creds.LazyCat != nil && creds.LazyCat.Username != "" && creds.LazyCat.Password != "" && creds.LazyCat.AutoRefresh {
		p, ok := providers.Get("lazycat")
		if ok {
			if lzProvider, isLz := p.(*lazycat.LazyCatProvider); isLz {
				totpCode := generateTOTPCode(creds.LazyCat.TOTPSecret)
				if newTok, need2FA, _, err := lzProvider.Login(ctx, creds.LazyCat.Username, creds.LazyCat.Password, totpCode); err == nil && !need2FA && newTok != "" {
					_ = config.SaveTokens("", "", newTok, "")
					config.Global.LazyCatToken = newTok
					lzProvider.SetToken(newTok)
					audit.Record("daemon", "token.auto_login", "lazycat", nil, "SUCCESS", "后台使用已保存账号密码自动登录懒猫云成功并注入 Token", 0)
				}
			}
		}
	}
}

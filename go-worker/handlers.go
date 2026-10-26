package main

import (
	"[github.com/gin-gonic/gin](https://github.com/gin-gonic/gin)"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

func SetupRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// 全量对齐业务及运维命名空间 /api
	api := r.Group("/api")
	{
		api.GET("/version", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"version":    "2.0.0-golang",
				"framework":  "gin",
				"engine":     "pure-go-embedded",
			})
		})

		// 新高可用运维接口：日志秒级直接获取，不再依赖 Nginx 磁盘回卷
		api.GET("/logs/app", func(c *gin.Context) {
			linesStr := c.DefaultQuery("lines", "500")
			lines, err := strconv.Atoi(linesStr)
			if err != nil || lines <= 0 {
				lines = 500
			}
			if lines > 5000 {
				lines = 5000
			}

			logPath := filepath.Join(".", "app.log")
			content, err := readLastLines(logPath, lines)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"lines":   lines,
				"content": content,
			})
		})
	}

	return r
}

func readLastLines(path string, maxLines int) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "[Log file empty or not initialized yet]", nil
	}
	defer file.Close()

	// 简单读取兜底表现
	stat, err := file.Stat()
	if err != nil {
		return "", err
	}
	size := stat.Size()
	if size == 0 {
		return "", nil
	}

	buf := make([]byte, size)
	_, err = file.Read(buf)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

/*
 * Copyright (c) 2023 Monimoto, UAB. All Rights Reserved.
 *
 *  This software contains the intellectual property of Monimoto, UAB. Use of
 *  this software and the intellectual property contained therein is expressly
 *  limited to the terms and conditions of the License Agreement under which
 *  it is provided by or on behalf of Monimoto, UAB.
 */

package logging

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// LoggerConfig holds the configuration for the logging middleware
type LoggerConfig struct {
	LogRequestBody  bool
	LogResponseBody bool
	IgnorePaths     []string
}

// NewLoggerConfig creates a new LoggerConfig with default settings.
// Pass in any paths you want to ignore or leave empty to use default paths.
func NewLoggerConfig(logRequestBody, logResponseBody bool, ignorePaths ...string) *LoggerConfig {
	defaultPaths := []string{"reports", "file", "health", "metrics", "swagger", "favicon", "static", "photo"}
	if len(ignorePaths) == 0 {
		ignorePaths = defaultPaths
	}
	return &LoggerConfig{
		LogRequestBody:  logRequestBody,
		LogResponseBody: logResponseBody,
		IgnorePaths:     ignorePaths,
	}
}

// GinRequestLogging is a middleware for logging requests
func GinRequestLogging(config *LoggerConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		clientIP := c.ClientIP()
		method := c.Request.Method
		statusCode := c.Writer.Status()

		if raw != "" {
			path = path + "?" + raw
		}

		defer printErrorOrEnd(c, start, statusCode, path)

		log.Tracef("[GIN] %d | %s | %s | %s",
			statusCode,
			path,
			method,
			clientIP,
		)

		loggableMethod := c.Request.Method == "POST" || c.Request.Method == "PUT"
		if config.LogRequestBody && loggableMethod {
			if isFileUploadRequest(c) {
				return
			}

			body := c.Request.Body
			b, err := io.ReadAll(body)
			if err == nil {
				log.Tracef("[REQ] body: %s", string(b))
			}
			// restore the io.ReadCloser to its original state
			c.Request.Body = io.NopCloser(bytes.NewBuffer(b))
		}
	}
}

func printErrorOrEnd(c *gin.Context, start time.Time, statusCode int, path string) {
	c.Next()

	comment := c.Errors.ByType(gin.ErrorTypePrivate).String()
	if comment != "" {
		log.Errorf("[ERR] %s", comment)
	}
	end := time.Now()
	latency := end.Sub(start)
	log.Tracef("[END] %d | %s | %s", statusCode, path, latency)
}

func isFileUploadRequest(c *gin.Context) bool {
	return strings.Contains(c.GetHeader("Content-Type"), "multipart/form-data")
}

type bodyLogWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w bodyLogWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// GinResponseLogging is a middleware for logging responses
func GinResponseLogging(config *LoggerConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !config.LogResponseBody {
			c.Next()
			return
		}

		loggableMethod := c.Request.Method == "POST" || c.Request.Method == "PUT" || c.Request.Method == "GET"
		if !loggableMethod || ignoreURL(config.IgnorePaths, c.Request.URL.Path) {
			c.Next()
			return
		}

		blw := &bodyLogWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
		c.Writer = blw
		c.Next()
		statusCode := c.Writer.Status()
		if statusCode >= http.StatusOK && statusCode < http.StatusNoContent {
			s := blw.body.String()
			if s == "null" {
				return
			}
			log.Tracef("[RES] %s", s)
		}
	}
}

// ignoreURL checks if the URL should be ignored based on the provided paths
func ignoreURL(ignorePaths []string, url string) bool {
	for _, path := range ignorePaths {
		if strings.Contains(url, path) {
			return true
		}
	}
	return false
}

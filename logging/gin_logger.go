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
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// LoggerConfig holds the configuration for the logging middleware
type LoggerConfig struct {
	LogRequestBody  bool
	LogResponseBody bool
	LogAssets       bool
	IgnorePaths     []string
}

// NewLoggerConfig creates a new LoggerConfig with default settings.
// Pass in any paths you want to ignore or leave empty to use default paths.
func NewLoggerConfig(logRequestBody, logResponseBody, logAssets bool, ignorePaths ...string) *LoggerConfig {
	defaultPaths := []string{"reports", "file", "swagger", "favicon", "static", "photo"}
	if len(ignorePaths) == 0 {
		ignorePaths = defaultPaths
	}
	return &LoggerConfig{
		LogRequestBody:  logRequestBody,
		LogResponseBody: logResponseBody,
		LogAssets:       logAssets,
		IgnorePaths:     ignorePaths,
	}
}

func Middleware(config *LoggerConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if ignoreURL(c, config) {
			c.Next()
			return
		}

		// for h, _ := range c.Request.Header {
		// 	log.Tracef("[RQH] %s: %s", h, c.GetHeader(h))
		// }
		start := time.Now()
		path := c.Request.URL.Path
		if c.Request.URL.RawQuery != "" {
			path = path + "?" + c.Request.URL.RawQuery
		}

		// get the response
		responseBodyWriter := &rewrittenBody{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
		c.Writer = responseBodyWriter
		c.Next()

		statusCode := c.Writer.Status()
		latency := time.Now().Sub(start)

		// for h, _ := range c.Writer.Header() {
		// 	log.Tracef("[RSH] %s: %s", h, c.GetHeader(h))
		// }
		// log a proof of request
		log.Tracef("[GIN] %d | %s | %s | %s | %s", statusCode, c.Request.Method, path, latency, c.ClientIP())

		// don't log request body when configured
		if !config.LogRequestBody {
			c.Next()
			return
		}

		// only these methods can contain request body
		if requestMethodHasBody(c.Request.Method) {
			requestBodyBytes, err := io.ReadAll(c.Request.Body)
			if err == nil {
				log.Tracef("[REQ] body: %s", string(requestBodyBytes))
			}
			// restore the original body to GIN's body reader and return the io.ReadCloser to its original state
			// for the next middleware
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBodyBytes))
		}

		if config.LogResponseBody && statusCode >= http.StatusOK && statusCode < http.StatusNoContent {
			s := responseBodyWriter.body.String()
			if s != "null" && s != "" && !strings.Contains(s, "<head") {
				log.Tracef("[RES] body: %s", s)
			}
		}

		// print request error
		ginErr := c.Errors.ByType(gin.ErrorTypePrivate).String()
		if ginErr != "" {
			log.Errorf("[ERRz] %s", ginErr)
		}
	}
}

func requestMethodHasBody(method string) bool {
	return method == "POST" || method == "PUT" || method == "PATCH"
}

type rewrittenBody struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w rewrittenBody) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// ignoreURL checks if the URL should be ignored based on the provided paths
func ignoreURL(c *gin.Context, config *LoggerConfig) bool {
	for _, path := range config.IgnorePaths {
		if strings.Contains(c.Request.URL.Path, path) {
			return true
		}
	}
	if !config.LogAssets && isStaticAsset(c) {
		return true
	}

	return false
}

// isStaticAsset checks if the request is for a static asset.
func isStaticAsset(c *gin.Context) bool {
	// Regular expression to match static file patterns
	staticAssetPattern := regexp.MustCompile(`\.(css|js|jpg|jpeg|png|gif|svg|ico|woff2|ttf|eot|html)$`)

	// Check if the request path matches the static asset pattern
	return staticAssetPattern.MatchString(c.Request.URL.Path)
}

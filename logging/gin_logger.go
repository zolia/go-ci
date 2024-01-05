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
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// Rule holds the configuration for the logging middleware
type Rule struct {
	Methods []string
	Pattern string
	// Request true means ignore request body, false means log it
	// when combined with Response true, it means ignore the entire request
	Config *Config
}

// Config holds the configuration for the logging middleware
type Config struct {
	LogRequestFact     bool
	LogRequestBody     bool
	LogRequestHeaders  bool
	LogResponseFact    bool
	LogResponseBody    bool
	LogResponseHeaders bool
	LogAssets          bool
	Rules              []Rule
}

var justFactsRule = JustFacts()

// DefaultRules are the default rules for logging paths
var DefaultRules = []Rule{
	{Methods: []string{"*"}, Pattern: `swagger`, Config: &justFactsRule},
	{Methods: []string{"*"}, Pattern: `favicon`},
}

// DefaultLogAllConfig is the default config for logging everything
var DefaultLogAllConfig = Config{
	LogRequestFact:     true,
	LogRequestBody:     true,
	LogRequestHeaders:  true,
	LogResponseFact:    true,
	LogResponseBody:    true,
	LogResponseHeaders: true,
	Rules:              DefaultRules,
}

// AllWithRules returns a config that logs everything with the given rules
func AllWithRules(rules ...Rule) Config {
	config := DefaultLogAllConfig
	config.Rules = append(config.Rules, rules...)

	return config
}

// JustFacts returns a config that only logs request and response facts
func JustFacts() Config {
	return Config{
		LogRequestFact: false,
		// response fact includes request fact
		LogResponseFact: true,
	}
}

// DefaultIgnoreRules are the default rules for ignoring paths
var DefaultIgnoreRules = []Rule{
	{Methods: []string{"GET"}, Pattern: `metrics`, Config: nil},
}

// Middleware is a gin middleware that logs requests and responses
func Middleware(config Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := getActiveOrDefaultConfig(c, config)
		log.Tracef("config: %+v", cfg)

		id := uuid.New()
		start := time.Now()

		endpointURL := c.Request.URL.Path
		if c.Request.URL.RawQuery != "" {
			endpointURL = endpointURL + "?" + c.Request.URL.RawQuery
		}

		if !cfg.LogAssets && isStaticAsset(c) {
			c.Next()
			return
		}

		if cfg.LogRequestHeaders {
			for h, _ := range c.Request.Header {
				log.Tracef("[REQH] [%s] %s: %s", id, h, c.GetHeader(h))
			}
		}

		if cfg.LogRequestFact {
			// request: log a proof of request
			log.Tracef("[REQ] [%s] %s | %s | %s", id, c.Request.Method, endpointURL, c.ClientIP())
		}

		if cfg.LogRequestBody {
			// only these methods can contain request body
			if requestMethodHasBody(c.Request.Method) {
				requestBodyBytes, err := io.ReadAll(c.Request.Body)
				body := bytes.TrimSpace(requestBodyBytes)
				if err == nil && len(body) > 0 {
					log.Tracef("[REQ] [%s] body: %s", id, string(requestBodyBytes))
				}
				// restore the original body to GIN's body reader and return the io.ReadCloser to its original state
				// for the next middleware or the route handler
				c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBodyBytes))
			}
		}

		// before Next, first part of response body logging.
		// build a new response writer to have our own copy of the response body
		// otherwise it would be lost after c.Next reading it
		responseBodyWriter := &rewrittenBody{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
		if cfg.LogResponseBody {
			c.Writer = responseBodyWriter
		}

		// process the request
		c.Next()

		statusCode := c.Writer.Status()

		if cfg.LogResponseFact {
			latency := time.Now().Sub(start)

			// request: log a proof of request
			log.Tracef("[RES] [%s] %d | %s | %s | %s | %s", id, statusCode, c.Request.Method, endpointURL, latency, c.ClientIP())
		}

		// after Next, second part of response body logging
		if cfg.LogResponseBody {
			// don't log response body when configured
			// or when the status code is not OK
			if statusCode >= http.StatusOK && statusCode < http.StatusNoContent {
				res := responseBodyWriter.body.String()
				// don't log empty responses
				// don't log html responses
				if res != "null" && res != "" && !strings.Contains(res, "<head") {
					log.Tracef("[RES] [%s] body: %s", id, res)
				}
			}

			// print request error
			ginErr := c.Errors.ByType(gin.ErrorTypePrivate).String()
			if ginErr != "" {
				log.Errorf("[RES] [%s] %s", id, ginErr)
			}
		}

		// purposely after response fact for better readability
		if cfg.LogResponseHeaders {
			for h, _ := range c.Writer.Header() {
				log.Tracef("[RESH] [%s] %s: %s", id, h, c.GetHeader(h))
			}
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

func getActiveOrDefaultConfig(c *gin.Context, config Config) Config {
	for _, rule := range config.Rules {
		matched, err := regexp.MatchString(rule.Pattern, c.Request.URL.Path)
		if err != nil {
			log.Errorf("error matching pattern: %v", err)
			continue
		}
		if matched && methodAllowed(c.Request.Method, rule.Methods) {
			// rule matched, but has no config
			if rule.Config == nil {
				// everything false
				return Config{}
			}
			return *rule.Config
		}
	}

	return config
}

func methodAllowed(given string, methods []string) bool {
	for _, method := range methods {
		if method == given || method == "*" {
			return true
		}
	}
	return false
}

// isStaticAsset checks if the request is for a static asset.
func isStaticAsset(c *gin.Context) bool {
	// Regular expression to match static file patterns
	staticAssetPattern := regexp.MustCompile(`\.(css|js|jpg|jpeg|png|gif|svg|ico|woff2|ttf|eot|html|favicon)$`)

	// Check if the request path matches the static asset pattern
	return staticAssetPattern.MatchString(c.Request.URL.Path)
}

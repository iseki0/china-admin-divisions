// Package ginhandler adapts the binary dataset endpoint to Gin.
package ginhandler

import (
	"github.com/gin-gonic/gin"
	division "github.com/iseki0/china-admin-divisions"
)

func New(loader *division.Loader, options ...division.HTTPOptions) gin.HandlerFunc {
	var config division.HTTPOptions
	if len(options) > 0 {
		config = options[0]
	}
	return gin.WrapH(division.Handler(loader, config))
}

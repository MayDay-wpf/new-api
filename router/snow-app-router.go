package router

import (
	"github.com/QuantumNous/new-api/controller"

	"github.com/gin-gonic/gin"
)

// SetSnowAppRouter 注册桌面客户端授权页。页面由 Go 直接渲染，
// 不依赖 web/dist 构建产物，便于私有化部署时直接使用。
func SetSnowAppRouter(router *gin.Engine) {
	router.GET("/snow/authorize", controller.SnowAppAuthorizePage)
}

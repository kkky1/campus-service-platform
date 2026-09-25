package httpapi

import (
	"github.com/gin-gonic/gin"

	"campus-service-platform/internal/middleware"
)

// NewRouter 路由注册：等价 MvcConfig 的拦截器与各 Controller 映射。
// 白名单外的所有路由要求登录；白名单路径见 middleware 包。
func NewRouter(h *Handlers) *gin.Engine {
	r := gin.New()
	r.Use(middleware.Recovery(h.App.Log))
	r.Use(middleware.TokenRefresh(h.App.RDB))
	r.Use(middleware.LoginAuth())

	// 用户
	r.POST("/user/code", h.SendCode)
	r.POST("/user/login", h.Login)
	r.POST("/user/logout", h.Logout)
	r.GET("/user/me", h.Me)
	r.GET("/user/info/:id", h.UserInfo)
	r.GET("/user/sign/count", h.SignCount)
	r.POST("/user/sign", h.Sign)
	r.GET("/user/:id", h.QueryUserById)

	// 上传
	r.POST("/upload/blog", h.UploadBlog)
	r.GET("/upload/blog/delete", h.DeleteBlogImg)

	// 店铺
	r.GET("/shop/:id", h.QueryShopById)
	r.POST("/shop", h.SaveShop)
	r.PUT("/shop", h.UpdateShop)
	r.GET("/shop/of/type", h.QueryShopByType)
	r.GET("/shop/of/name", h.QueryShopByName)

	// 店铺类型
	r.GET("/shop-type/list", h.QueryShopTypeList)

	// 动态
	r.POST("/blog", h.SaveBlog)
	r.PUT("/blog/like/:id", h.LikeBlog)
	r.GET("/blog/of/me", h.QueryMyBlog)
	r.GET("/blog/hot", h.QueryHotBlog)
	r.GET("/blog/likes/:id", h.QueryBlogLikes)
	r.GET("/blog/of/user", h.QueryBlogByUserId)
	r.GET("/blog/of/follow", h.QueryBlogOfFollow)
	r.GET("/blog/:id", h.QueryBlogById)

	// 关注
	r.PUT("/follow/:id/:isFollow", h.Follow)
	r.GET("/follow/or/not/:id", h.IsFollow)
	r.GET("/follow/common/:id", h.FollowCommons)

	// 券
	r.POST("/voucher", h.AddVoucher)
	r.POST("/voucher/seckill", h.AddSeckillVoucher)
	r.GET("/voucher/list/:shopId", h.QueryVoucherOfShop)

	// 秒杀下单
	r.POST("/voucher-order/seckill/:id", h.SeckillVoucher)

	// RAG 知识库（可选模块）
	if h.Rag != nil {
		rag := r.Group("/rag")
		{
			rag.POST("/kb", h.Rag.CreateKB)
			rag.GET("/kb/list", h.Rag.ListKB)
			rag.DELETE("/kb/:id", h.Rag.DeleteKB)
			rag.POST("/doc/upload", h.Rag.UploadDoc)
			rag.GET("/doc/list", h.Rag.ListDocs)
			rag.GET("/doc/:id/chunks", h.Rag.DocChunks)
			rag.DELETE("/doc/:id", h.Rag.DeleteDoc)
			rag.POST("/chat", h.Rag.Chat)
			rag.POST("/retrieve", h.Rag.Retrieve)
			rag.POST("/eval/run", h.Rag.EvalRun)
			rag.GET("/eval/:id", h.Rag.EvalGet)
		}
	}

	return r
}

// Package httpapi HTTP 处理器与路由注册。
package httpapi

import (
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"campus-service-platform/internal/middleware"
	"campus-service-platform/internal/pkg/dto"
	"campus-service-platform/internal/repo"
	"campus-service-platform/internal/service"
)

// Handlers 持有 App 的处理器集合。
type Handlers struct {
	App *service.App
	Rag *RagHandlers // 可选：RAG 模块处理器
}

// WithRag 注入 RAG 处理器。
func (h *Handlers) WithRag(r *RagHandlers) *Handlers {
	h.Rag = r
	return h
}

// New 构造处理器。
func New(app *service.App) *Handlers { return &Handlers{App: app} }

// --- 用户 / 认证 ---

// SendCode POST /user/code?phone=
func (h *Handlers) SendCode(c *gin.Context) {
	c.JSON(200, h.App.SendCode(c.Request.Context(), c.Query("phone")))
}

// Login POST /user/login
// 注意：code 允许缺省（验证码校验关闭时前端可留空）；缺省时按空字符串处理。
func (h *Handlers) Login(c *gin.Context) {
	var body struct {
		Phone    *string `json:"phone"`
		Code     *string `json:"code"`
		Password *string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Phone == nil {
		c.JSON(200, dto.Fail("手机号不能为空"))
		return
	}
	code := ""
	if body.Code != nil {
		code = *body.Code
	}
	c.JSON(200, h.App.Login(c.Request.Context(), *body.Phone, code))
}

// Logout POST /user/logout（原行为：恒"功能未完成"）
func (h *Handlers) Logout(c *gin.Context) {
	c.JSON(200, dto.Fail("功能未完成"))
}

// Me GET /user/me
func (h *Handlers) Me(c *gin.Context) {
	u := middleware.CurrentUser(c)
	if u == nil {
		c.JSON(200, dto.Ok())
		return
	}
	c.JSON(200, dto.OkData(u))
}

// UserInfo GET /user/info/{id}
func (h *Handlers) UserInfo(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	c.JSON(200, h.App.QueryUserInfo(c.Request.Context(), id))
}

// QueryUserById GET /user/{id}
func (h *Handlers) QueryUserById(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	c.JSON(200, h.App.QueryUserById(c.Request.Context(), id))
}

// UpdateProfile PUT /user/profile（昵称/头像）
func (h *Handlers) UpdateProfile(c *gin.Context) {
	u := middleware.CurrentUser(c)
	var body struct {
		NickName *string `json:"nickName"`
		Icon     *string `json:"icon"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || u == nil || u.Id == nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	res := h.App.UpdateProfile(c.Request.Context(), *u.Id, body.NickName, body.Icon)
	if res.Success {
		h.App.RefreshSession(c.Request.Context(), c.GetHeader("authorization"), body.NickName, body.Icon)
	}
	c.JSON(200, res)
}

// UpdateUserInfo PUT /user/info（介绍/性别/校区/生日）
func (h *Handlers) UpdateUserInfo(c *gin.Context) {
	u := middleware.CurrentUser(c)
	var body struct {
		Introduce *string `json:"introduce"`
		Gender    *bool   `json:"gender"`
		City      *string `json:"city"`
		Birthday  *string `json:"birthday"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || u == nil || u.Id == nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	c.JSON(200, h.App.UpsertUserInfo(c.Request.Context(), *u.Id, body.Introduce, body.Gender, body.City, body.Birthday))
}

// Sign POST /user/sign
func (h *Handlers) Sign(c *gin.Context) {
	u := middleware.CurrentUser(c)
	if u == nil || u.Id == nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	c.JSON(200, h.App.Sign(c.Request.Context(), *u.Id))
}

// SignCount GET /user/sign/count
func (h *Handlers) SignCount(c *gin.Context) {
	u := middleware.CurrentUser(c)
	if u == nil || u.Id == nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	c.JSON(200, h.App.SignCount(c.Request.Context(), *u.Id))
}

// --- 上传 ---

// UploadBlog POST /upload/blog
func (h *Handlers) UploadBlog(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	f, err := file.Open()
	if err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	c.JSON(200, h.App.UploadImage(c.Request.Context(), file.Filename, data))
}

// DeleteBlogImg GET /upload/blog/delete?name=
func (h *Handlers) DeleteBlogImg(c *gin.Context) {
	c.JSON(200, h.App.DeleteImage(c.Request.Context(), c.Query("name")))
}

// --- 店铺 ---

// QueryShopById GET /shop/{id}
func (h *Handlers) QueryShopById(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	c.JSON(200, h.App.QueryShopById(c.Request.Context(), id))
}

// SaveShop POST /shop
func (h *Handlers) SaveShop(c *gin.Context) {
	var s repo.Shop
	if err := c.ShouldBindJSON(&s); err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	c.JSON(200, h.App.SaveShop(c.Request.Context(), &s))
}

// UpdateShop PUT /shop
func (h *Handlers) UpdateShop(c *gin.Context) {
	var s repo.Shop
	if err := c.ShouldBindJSON(&s); err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	c.JSON(200, h.App.UpdateShop(c.Request.Context(), &s))
}

// QueryShopByType GET /shop/of/type
func (h *Handlers) QueryShopByType(c *gin.Context) {
	typeId, err := strconv.ParseInt(c.Query("typeId"), 10, 64)
	if err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	current := atoiDefault(c.Query("current"), 1)
	var x, y *float64
	if v, e := strconv.ParseFloat(c.Query("x"), 64); e == nil {
		x = &v
	}
	if v, e := strconv.ParseFloat(c.Query("y"), 64); e == nil {
		y = &v
	}
	c.JSON(200, h.App.QueryShopByType(c.Request.Context(), typeId, current, x, y, c.Query("sortBy")))
}

// QueryShopByName GET /shop/of/name
func (h *Handlers) QueryShopByName(c *gin.Context) {
	current := atoiDefault(c.Query("current"), 1)
	c.JSON(200, h.App.QueryShopByName(c.Request.Context(), c.Query("name"), current))
}

// QueryShopTypeList GET /shop-type/list
func (h *Handlers) QueryShopTypeList(c *gin.Context) {
	c.JSON(200, h.App.QueryShopTypeList(c.Request.Context()))
}

// --- 动态 ---

// SaveBlog POST /blog
func (h *Handlers) SaveBlog(c *gin.Context) {
	u := middleware.CurrentUser(c)
	if u == nil || u.Id == nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	var b repo.Blog
	if err := c.ShouldBindJSON(&b); err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	c.JSON(200, h.App.SaveBlog(c.Request.Context(), *u.Id, &b))
}

// LikeBlog PUT /blog/like/{id}
func (h *Handlers) LikeBlog(c *gin.Context) {
	u := middleware.CurrentUser(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || u == nil || u.Id == nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	c.JSON(200, h.App.UpdateLike(c.Request.Context(), *u.Id, id))
}

// QueryHotBlog GET /blog/hot
func (h *Handlers) QueryHotBlog(c *gin.Context) {
	c.JSON(200, h.App.QueryHotBlog(c.Request.Context(), atoiDefault(c.Query("current"), 1), middleware.CurrentUser(c)))
}

// QueryBlogById GET /blog/{id}
func (h *Handlers) QueryBlogById(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	c.JSON(200, h.App.QueryBlogById(c.Request.Context(), id, middleware.CurrentUser(c)))
}

// QueryBlogLikes GET /blog/likes/{id}
func (h *Handlers) QueryBlogLikes(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	c.JSON(200, h.App.QueryBlogLikes(c.Request.Context(), id))
}

// QueryMyBlog GET /blog/of/me
func (h *Handlers) QueryMyBlog(c *gin.Context) {
	u := middleware.CurrentUser(c)
	if u == nil || u.Id == nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	c.JSON(200, h.App.QueryBlogOfMe(c.Request.Context(), *u.Id, atoiDefault(c.Query("current"), 1)))
}

// QueryBlogByUserId GET /blog/of/user
func (h *Handlers) QueryBlogByUserId(c *gin.Context) {
	id, err := strconv.ParseInt(c.Query("id"), 10, 64)
	if err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	c.JSON(200, h.App.QueryBlogOfUser(c.Request.Context(), id, atoiDefault(c.Query("current"), 1)))
}

// QueryBlogOfFollow GET /blog/of/follow
func (h *Handlers) QueryBlogOfFollow(c *gin.Context) {
	u := middleware.CurrentUser(c)
	max, err := strconv.ParseInt(c.Query("lastId"), 10, 64)
	if err != nil || u == nil || u.Id == nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	c.JSON(200, h.App.QueryBlogOfFollow(c.Request.Context(), *u.Id, max, atoiDefault(c.Query("offset"), 0), u))
}

// --- 关注 ---

// Follow PUT /follow/{id}/{isFollow}
func (h *Handlers) Follow(c *gin.Context) {
	u := middleware.CurrentUser(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || u == nil || u.Id == nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	isFollow := c.Param("isFollow") == "true"
	c.JSON(200, h.App.Follow(c.Request.Context(), *u.Id, id, isFollow))
}

// IsFollow GET /follow/or/not/{id}
func (h *Handlers) IsFollow(c *gin.Context) {
	u := middleware.CurrentUser(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || u == nil || u.Id == nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	c.JSON(200, h.App.IsFollow(c.Request.Context(), *u.Id, id))
}

// FollowCommons GET /follow/common/{id}
func (h *Handlers) FollowCommons(c *gin.Context) {
	u := middleware.CurrentUser(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || u == nil || u.Id == nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	c.JSON(200, h.App.FollowCommons(c.Request.Context(), *u.Id, id))
}

// --- 券与秒杀 ---

// AddVoucher POST /voucher（普通券）
func (h *Handlers) AddVoucher(c *gin.Context) {
	var v repo.Voucher
	if err := c.ShouldBindJSON(&v); err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	c.JSON(200, h.App.AddPlainVoucher(c.Request.Context(), &v))
}

// AddSeckillVoucher POST /voucher/seckill（秒杀券）
func (h *Handlers) AddSeckillVoucher(c *gin.Context) {
	var v repo.Voucher
	if err := c.ShouldBindJSON(&v); err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	c.JSON(200, h.App.AddSeckillVoucher(c.Request.Context(), &v))
}

// QueryVoucherOfShop GET /voucher/list/{shopId}
func (h *Handlers) QueryVoucherOfShop(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("shopId"), 10, 64)
	if err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	c.JSON(200, h.App.QueryVoucherOfShop(c.Request.Context(), id))
}

// SeckillVoucher POST /voucher-order/seckill/{id}（需登录，路径在白名单内，handler 内校验）
func (h *Handlers) SeckillVoucher(c *gin.Context) {
	u := middleware.CurrentUser(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	if u == nil || u.Id == nil {
		c.JSON(200, dto.Fail("服务器异常"))
		return
	}
	c.JSON(200, h.App.SeckillVoucher(c.Request.Context(), *u.Id, id))
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

var _ = http.StatusOK

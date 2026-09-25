package httpapi

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"campus-service-platform/internal/pkg/dto"
	"campus-service-platform/internal/pkg/rds"
	"campus-service-platform/internal/repo"
	"campus-service-platform/internal/service"
)

type fakePub struct{}

func (fakePub) PublishSeckillOrder(_ context.Context, _ service.SeckillOrderMsg) error { return nil }

// fullEnv 全模型 sqlite + miniredis 的完整路由。
func fullEnv(t *testing.T) (*gin.Engine, *gorm.DB, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&repo.User{}, &repo.UserInfo{}, &repo.Shop{}, &repo.ShopType{},
		&repo.Blog{}, &repo.BlogComments{}, &repo.Follow{},
		&repo.Voucher{}, &repo.SeckillVoucher{}, &repo.VoucherOrder{},
	); err != nil {
		t.Fatal(err)
	}
	app := service.New(db, rdb, fakePub{}, nil, t.TempDir())
	gin.SetMode(gin.TestMode)
	return NewRouter(New(app)), db, mr
}

func loginToken(t *testing.T, r *gin.Engine, mr *miniredis.Miniredis, phone string) (string, int64) {
	t.Helper()
	mr.Set(rds.LoginCodeKey+phone, "123456")
	_, m := doJSON(t, r, "POST", "/user/login", `{"phone":"`+phone+`","code":"123456"}`, nil)
	token, _ := m["data"].(string)
	idStr := mr.HGet(rds.LoginUserKey+token, "id")
	var id int64
	json.Unmarshal([]byte(idStr), &id)
	return token, id
}

// --- 3.1 用户资料 ---

func TestUserInfoContract(t *testing.T) {
	r, db, mr := fullEnv(t)
	token, _ := loginToken(t, r, mr, "13800000011")
	// 无资料 → success 无 data
	_, m := doJSON(t, r, "GET", "/user/info/1", "", map[string]string{"authorization": token})
	if m["success"] != true {
		t.Fatalf("无资料响应: %v", m)
	}
	assertKeys(t, m, "success")
	// 有资料 → 字段集不含 createTime/updateTime
	db.Create(&repo.UserInfo{UserId: int64P(1), City: strP("杭州"), Credits: intP(10)})
	_, m = doJSON(t, r, "GET", "/user/info/1", "", map[string]string{"authorization": token})
	assertKeys(t, m, "success", "data")
	d := m["data"].(map[string]any)
	assertKeys(t, d, "userId", "city", "credits")
	if _, has := d["createTime"]; has {
		t.Fatalf("createTime 应省略: %v", d)
	}
}

func TestUserSummaryContract(t *testing.T) {
	r, db, mr := fullEnv(t)
	token, _ := loginToken(t, r, mr, "13800000012")
	_, m := doJSON(t, r, "GET", "/user/999", "", map[string]string{"authorization": token})
	if m["success"] != true {
		t.Fatalf("无用户响应: %v", m)
	}
	assertKeys(t, m, "success")
	db.Create(&repo.User{Id: int64P(9), Phone: strP("p"), NickName: strP("小明"), Icon: strP("")})
	_, m = doJSON(t, r, "GET", "/user/9", "", map[string]string{"authorization": token})
	d := m["data"].(map[string]any)
	assertKeys(t, d, "id", "nickName", "icon")
	if d["icon"] != "" {
		t.Fatalf("空串 icon 应输出: %v", d)
	}
}

// --- 4.3 / 4.5 店铺契约 ---

func TestShopWriteContract(t *testing.T) {
	r, db, _ := fullEnv(t)
	// 新增
	_, m := doJSON(t, r, "POST", "/shop", `{"name":"一食堂","typeId":1}`, nil)
	if m["success"] != true || m["data"] == nil {
		t.Fatalf("新增店铺失败: %v", m)
	}
	// 更新（含删缓存）
	_, m = doJSON(t, r, "PUT", "/shop", `{"id":1,"name":"二食堂"}`, nil)
	if m["success"] != true {
		t.Fatalf("更新失败: %v", m)
	}
	var s repo.Shop
	db.First(&s, 1)
	if *s.Name != "二食堂" {
		t.Fatalf("更新未生效: %v", s)
	}
	// 名称搜索（页大小 10）
	db.Create(&repo.Shop{Id: int64P(2), Name: strP("食堂2号")})
	_, m = doJSON(t, r, "GET", "/shop/of/name?name=食堂&current=1", "", nil)
	arr := m["data"].([]any)
	if len(arr) != 2 {
		t.Fatalf("名称搜索结果 = %d, want 2", len(arr))
	}
}

func TestShopTypeListContract(t *testing.T) {
	r, db, _ := fullEnv(t)
	_, m := doJSON(t, r, "GET", "/shop-type/list", "", nil)
	if m["errorMsg"] != "没有分类数据" {
		t.Fatalf("空分类文案: %v", m)
	}
	db.Create(&repo.ShopType{Id: int64P(1), Name: strP("美食"), Sort: intP(1)})
	_, m = doJSON(t, r, "GET", "/shop-type/list", "", nil)
	arr := m["data"].([]any)
	if len(arr) != 1 {
		t.Fatalf("分类列表: %v", m)
	}
	// 类型 JSON 不含 createTime/updateTime
	d := arr[0].(map[string]any)
	assertKeys(t, d, "id", "name", "sort")
}

// --- 5.2 动态契约 ---

func TestBlogContract(t *testing.T) {
	r, db, mr := fullEnv(t)
	token, uid := loginToken(t, r, mr, "13800000001")
	// 发布
	_, m := doJSON(t, r, "POST", "/blog", `{"title":"标题","content":"内容"}`, map[string]string{"authorization": token})
	if m["success"] != true {
		t.Fatalf("发布失败: %v", m)
	}
	// sqlite 无 MySQL 列默认值，模拟 liked DEFAULT 0
	db.Model(&repo.Blog{}).Where("id = 1").Update("liked", 0)
	// 点赞
	_, m = doJSON(t, r, "PUT", "/blog/like/1", "", map[string]string{"authorization": token})
	if m["success"] != true {
		t.Fatalf("点赞失败: %v", m)
	}
	var b repo.Blog
	db.First(&b, 1)
	if b.Liked == nil || *b.Liked != 1 {
		t.Fatalf("liked 未加一: %v", b.Liked)
	}
	// 再点取消
	doJSON(t, r, "PUT", "/blog/like/1", "", map[string]string{"authorization": token})
	db.First(&b, 1)
	if b.Liked == nil || *b.Liked != 0 {
		t.Fatalf("取消点赞未减一: %v", b.Liked)
	}
	// 热门榜（未登录 isLike 省略）
	_, m = doJSON(t, r, "GET", "/blog/hot?current=1", "", nil)
	arr := m["data"].([]any)
	if len(arr) != 1 {
		t.Fatalf("热门榜: %v", m)
	}
	first := arr[0].(map[string]any)
	for _, k := range []string{"id", "title", "name", "icon"} {
		if _, has := first[k]; !has {
			t.Fatalf("热门榜缺 %s: %v", k, first)
		}
	}
	// 详情（携带 token → isLike=false 输出）
	_, m = doJSON(t, r, "GET", "/blog/1", "", map[string]string{"authorization": token})
	d := m["data"].(map[string]any)
	if d["isLike"] != false {
		t.Fatalf("isLike 应为 false: %v", d)
	}
	// 详情不存在
	_, m = doJSON(t, r, "GET", "/blog/999", "", map[string]string{"authorization": token})
	if m["errorMsg"] != "博客不存在" {
		t.Fatalf("不存在文案: %v", m)
	}
	// 我的动态
	_, m = doJSON(t, r, "GET", "/blog/of/me?current=1", "", map[string]string{"authorization": token})
	arr = m["data"].([]any)
	if len(arr) != 1 {
		t.Fatalf("我的动态: %v", m)
	}
	// 点赞榜
	_, m = doJSON(t, r, "GET", "/blog/likes/1", "", map[string]string{"authorization": token})
	if m["data"] == nil {
		t.Fatalf("点赞榜: %v", m)
	}
	_ = uid
}

// --- 5.4 关注契约 ---

func TestFollowContract(t *testing.T) {
	r, db, mr := fullEnv(t)
	db.Create(&repo.User{Id: int64P(2), Phone: strP("p2"), NickName: strP("小红"), Icon: strP("")})
	t1, u1 := loginToken(t, r, mr, "13800000002")
	// 关注
	_, m := doJSON(t, r, "PUT", "/follow/2/true", "", map[string]string{"authorization": t1})
	if m["success"] != true {
		t.Fatalf("关注失败: %v", m)
	}
	// 是否关注
	_, m = doJSON(t, r, "GET", "/follow/or/not/2", "", map[string]string{"authorization": t1})
	if m["data"] != true {
		t.Fatalf("是否关注: %v", m)
	}
	// 取关
	doJSON(t, r, "PUT", "/follow/2/false", "", map[string]string{"authorization": t1})
	_, m = doJSON(t, r, "GET", "/follow/or/not/2", "", map[string]string{"authorization": t1})
	if m["data"] != false {
		t.Fatalf("取关后状态: %v", m)
	}
	// 共同关注
	db.Exec("INSERT INTO tb_follow (user_id, follow_user_id) VALUES (?, ?)", u1, 3)
	db.Exec("INSERT INTO tb_follow (user_id, follow_user_id) VALUES (?, ?)", 2, 3)
	mr.SAdd("follows:"+jsonNum(u1), "3")
	mr.SAdd("follows:2", "3")
	db.Create(&repo.User{Id: int64P(3), Phone: strP("p3"), NickName: strP("小刚"), Icon: strP("")})
	_, m = doJSON(t, r, "GET", "/follow/common/2", "", map[string]string{"authorization": t1})
	arr := m["data"].([]any)
	if len(arr) != 1 {
		t.Fatalf("共同关注: %v", m)
	}
}

func jsonNum(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// --- 6.2 券契约 ---

func TestVoucherContract(t *testing.T) {
	r, db, mr := fullEnv(t)
	// 发布（含 stock）
	body := `{"shopId":1,"title":"福利券","subTitle":"学生认证","payValue":5000,"actualValue":10000,"type":1,"status":1,"stock":100,"beginTime":"2026-09-01T10:00:00","endTime":"2026-12-31T10:00:00"}`
	_, m := doJSON(t, r, "POST", "/voucher/seckill", body, nil)
	if m["success"] != true || m["data"] == nil {
		t.Fatalf("发布秒杀券失败: %v", m)
	}
	// Redis 库存预载
	if v, _ := mr.Get(rds.SeckillStockKey + "1"); v != "100" {
		t.Fatalf("库存未预载: %q", v)
	}
	// 列表（LEFT JOIN 附带 stock/beginTime/endTime）
	_, m = doJSON(t, r, "GET", "/voucher/list/1", "", nil)
	arr := m["data"].([]any)
	if len(arr) != 1 {
		t.Fatalf("券列表: %v", m)
	}
	d := arr[0].(map[string]any)
	if d["stock"] != float64(100) || d["beginTime"] == nil {
		t.Fatalf("券列表 JOIN 字段: %v", d)
	}
	// 无 stock 发布 → 服务器异常且无记录（对齐原 NPE 回滚行为）
	_, m = doJSON(t, r, "POST", "/voucher", `{"shopId":1,"title":"x","status":1}`, nil)
	if m["errorMsg"] != "服务器异常" {
		t.Fatalf("无 stock 发布: %v", m)
	}
	var cnt int64
	db.Model(&repo.Voucher{}).Where("title = ?", "x").Count(&cnt)
	if cnt != 0 {
		t.Fatalf("事务未回滚: %d", cnt)
	}
}

var _ = time.Second
var _ = dto.Ok

func strP(s string) *string { return &s }
func intP(v int) *int       { return &v }
func int64P(v int64) *int64 { return &v }

package service

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"

	"campus-service-platform/internal/pkg/dto"
	"campus-service-platform/internal/pkg/rds"
	"campus-service-platform/internal/repo"
)

func memberScore(m string, s float64) redis.Z { return redis.Z{Score: s, Member: m} }

func dtoUser(id int64) *dto.UserDTO { return &dto.UserDTO{Id: int64Ptr(id)} }

// 发布动态 → 粉丝 feed 写入（5.1）
func TestSaveBlogPushesFeed(t *testing.T) {
	app, _ := newAppEnv(t)
	ctx := context.Background()
	// 两个粉丝关注了作者 1
	app.DB.Create(&repo.Follow{UserId: int64Ptr(2), FollowUserId: int64Ptr(1)})
	app.DB.Create(&repo.Follow{UserId: int64Ptr(3), FollowUserId: int64Ptr(1)})
	b := repo.Blog{Title: strPtr("新动态"), Content: strPtr("内容")}
	res := app.SaveBlog(ctx, 1, &b)
	if !res.Success {
		t.Fatalf("发布失败: %v", res)
	}
	blogId := res.Data.(int64)
	for _, fid := range []int64{2, 3} {
		n, err := app.RDB.ZCard(ctx, rds.FeedKey+itoa(fid)).Result()
		if err != nil || n != 1 {
			t.Fatalf("粉丝 %d feed 数量 = %d, want 1", fid, n)
		}
		members, _ := app.RDB.ZRange(ctx, rds.FeedKey+itoa(fid), 0, -1).Result()
		if len(members) != 1 || members[0] != itoa(blogId) {
			t.Fatalf("粉丝 %d feed 内容不对: %v", fid, members)
		}
	}
}

// B7 修复回归：我的动态按 id 倒序（超过一页时最新动态必须在第一页）
func TestQueryBlogOfMeNewestFirst(t *testing.T) {
	app, _ := newAppEnv(t)
	ctx := context.Background()
	for i := 1; i <= 15; i++ {
		id := int64(i)
		app.DB.Create(&repo.Blog{Id: &id, UserId: int64Ptr(7), Title: strPtr("b"), Images: strPtr(""), Content: strPtr("c")})
	}
	res := app.QueryBlogOfMe(ctx, 7, 1)
	list := res.Data.([]repo.Blog)
	if len(list) != 10 {
		t.Fatalf("第一页应为 10 条: %d", len(list))
	}
	if list[0].Id == nil || *list[0].Id != 15 {
		t.Fatalf("第一页首条应为最新 id=15: %v", list[0].Id)
	}
	if list[9].Id == nil || *list[9].Id != 6 {
		t.Fatalf("第一页末条应为 id=6: %v", list[9].Id)
	}
}

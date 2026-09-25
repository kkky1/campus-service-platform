package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"campus-service-platform/internal/pkg/dto"
	"campus-service-platform/internal/pkg/rds"
	"campus-service-platform/internal/repo"
)

// SaveBlog 发布动态：插入 + 推送所有粉丝 feed ZSET。
func (a *App) SaveBlog(ctx context.Context, userId int64, b *repo.Blog) dto.Result {
	b.UserId = &userId
	cols := blogColumns(b)
	if err := a.DB.WithContext(ctx).Select(cols).Create(b).Error; err != nil {
		a.Log.Error("发布动态失败", "err", err)
		return dto.Fail("服务器异常")
	}
	// 推送给所有粉丝
	var fans []repo.Follow
	if err := a.DB.WithContext(ctx).Where("follow_user_id = ?", userId).Find(&fans).Error; err != nil {
		a.Log.Error("查询粉丝失败", "err", err)
		return dto.Fail("服务器异常")
	}
	score := float64(time.Now().UnixMilli())
	member := fmt.Sprint(*b.Id)
	pipe := a.RDB.Pipeline()
	for _, f := range fans {
		if f.UserId != nil {
			pipe.ZAdd(ctx, fmt.Sprintf("%s%d", rds.FeedKey, *f.UserId), redis.Z{Score: score, Member: member})
		}
	}
	if _, err := pipe.Exec(ctx); err != nil {
		a.Log.Error("推送 feed 失败", "err", err)
		return dto.Fail("服务器异常")
	}
	return dto.OkData(*b.Id)
}

func blogColumns(b *repo.Blog) []string {
	var cols []string
	if b.ShopId != nil {
		cols = append(cols, "shop_id")
	}
	if b.UserId != nil {
		cols = append(cols, "user_id")
	}
	if b.Title != nil {
		cols = append(cols, "title")
	}
	if b.Images != nil {
		cols = append(cols, "images")
	}
	if b.Content != nil {
		cols = append(cols, "content")
	}
	return cols
}

// UpdateLike 点赞切换：ZSCORE 判存在，liked±1 + ZADD/ZREM。
func (a *App) UpdateLike(ctx context.Context, userId, blogId int64) dto.Result {
	key := fmt.Sprintf("%s%d", rds.BlogLikedKey, blogId)
	member := strconv.FormatInt(userId, 10)
	_, err := a.RDB.ZScore(ctx, key, member).Result()
	liked := errors.Is(err, redis.Nil)
	if err != nil && !liked {
		return dto.Fail("服务器异常")
	}
	delta := 1
	if !liked {
		delta = -1
	}
	if e := a.DB.WithContext(ctx).Model(&repo.Blog{}).Where("id = ?", blogId).
		Update("liked", gorm.Expr("liked + ?", delta)).Error; e != nil {
		return dto.Fail("服务器异常")
	}
	if liked {
		a.RDB.ZAdd(ctx, key, redis.Z{Score: float64(time.Now().UnixMilli()), Member: member})
	} else {
		a.RDB.ZRem(ctx, key, member)
	}
	return dto.Ok()
}

// enrichBlog 附加作者 name/icon 与 isLike。
func (a *App) enrichBlogs(ctx context.Context, blogs []repo.Blog, user *dto.UserDTO) []repo.Blog {
	for i := range blogs {
		b := &blogs[i]
		if b.UserId == nil {
			continue
		}
		var u repo.User
		if err := a.DB.WithContext(ctx).First(&u, *b.UserId).Error; err == nil {
			b.Name = u.NickName
			b.Icon = u.Icon
		}
		if user != nil {
			// Java: setIsLike(score != null) —— 无论是否赞过都输出布尔值
			_, err := a.RDB.ZScore(ctx, fmt.Sprintf("%s%d", rds.BlogLikedKey, *b.Id), strconv.FormatInt(*user.Id, 10)).Result()
			v := err == nil
			b.IsLike = &v
		}
	}
	return blogs
}

// QueryHotBlog 热门榜：liked 倒序，页大小 10。
func (a *App) QueryHotBlog(ctx context.Context, current int, user *dto.UserDTO) dto.Result {
	var blogs []repo.Blog
	if err := a.DB.WithContext(ctx).Order("liked DESC").
		Offset((current - 1) * rds.MaxPageSize).Limit(rds.MaxPageSize).
		Find(&blogs).Error; err != nil {
		return dto.Fail("服务器异常")
	}
	if blogs == nil {
		blogs = []repo.Blog{}
	}
	return dto.OkData(a.enrichBlogs(ctx, blogs, user))
}

// QueryBlogById 详情。
func (a *App) QueryBlogById(ctx context.Context, id int64, user *dto.UserDTO) dto.Result {
	var b repo.Blog
	err := a.DB.WithContext(ctx).First(&b, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return dto.Fail("博客不存在")
	}
	if err != nil {
		return dto.Fail("服务器异常")
	}
	list := a.enrichBlogs(ctx, []repo.Blog{b}, user)
	return dto.OkData(list[0])
}

// QueryBlogLikes 点赞榜前 5（按 ZSET 顺序，FIELD 排序取用户）。
func (a *App) QueryBlogLikes(ctx context.Context, blogId int64) dto.Result {
	key := fmt.Sprintf("%s%d", rds.BlogLikedKey, blogId)
	members, err := a.RDB.ZRange(ctx, key, 0, rds.TopLikers-1).Result()
	if err != nil || len(members) == 0 {
		return dto.OkData([]dto.UserDTO{})
	}
	ids := make([]int64, 0, len(members))
	for _, m := range members {
		if id, e := strconv.ParseInt(m, 10, 64); e == nil {
			ids = append(ids, id)
		}
	}
	var users []repo.User
	if err := a.DB.WithContext(ctx).Where("id IN ?", ids).
		Order("field(id, " + joinInt64(ids) + ")").
		Find(&users).Error; err != nil {
		return dto.Fail("服务器异常")
	}
	out := make([]dto.UserDTO, 0, len(users))
	for i := range users {
		out = append(out, userToDTO(&users[i]))
	}
	return dto.OkData(out)
}

// QueryBlogOfMe 我的动态（页大小 10，按发布时间倒序，无附加字段）。
func (a *App) QueryBlogOfMe(ctx context.Context, userId int64, current int) dto.Result {
	var blogs []repo.Blog
	if err := a.DB.WithContext(ctx).Where("user_id = ?", userId).
		Order("id DESC").
		Offset((current - 1) * rds.MaxPageSize).Limit(rds.MaxPageSize).
		Find(&blogs).Error; err != nil {
		return dto.Fail("服务器异常")
	}
	if blogs == nil {
		blogs = []repo.Blog{}
	}
	return dto.OkData(blogs)
}

// QueryBlogOfUser 指定用户动态（页大小 10，无附加字段）。
func (a *App) QueryBlogOfUser(ctx context.Context, userId int64, current int) dto.Result {
	return a.QueryBlogOfMe(ctx, userId, current)
}

// QueryBlogOfFollow 关注 feed 滚动流：ZREVRANGEBYSCORE 取 2 条 + 同分 offset。
func (a *App) QueryBlogOfFollow(ctx context.Context, userId int64, max int64, offset int, user *dto.UserDTO) dto.Result {
	key := fmt.Sprintf("%s%d", rds.FeedKey, userId)
	tuples, err := a.RDB.ZRevRangeByScoreWithScores(ctx, key, &redis.ZRangeBy{
		Min:    "0",
		Max:    strconv.FormatInt(max, 10),
		Offset: int64(offset),
		Count:  rds.FeedPageCount,
	}).Result()
	if err != nil || len(tuples) == 0 {
		return dto.Ok()
	}
	ids := make([]int64, 0, len(tuples))
	var minTime int64
	os := 1
	for _, tup := range tuples {
		if id, e := strconv.ParseInt(tup.Member.(string), 10, 64); e == nil {
			ids = append(ids, id)
		}
		ts := int64(tup.Score)
		if ts == minTime {
			os++
		} else {
			minTime = ts
			os = 1
		}
	}
	var blogs []repo.Blog
	if err := a.DB.WithContext(ctx).Where("id IN ?", ids).
		Order("field(id, " + joinInt64(ids) + ")").
		Find(&blogs).Error; err != nil {
		return dto.Fail("服务器异常")
	}
	if blogs == nil {
		blogs = []repo.Blog{}
	}
	return dto.OkData(dto.ScrollResult{
		List:    a.enrichBlogs(ctx, blogs, user),
		MinTime: &minTime,
		Offset:  &os,
	})
}

// Follow 关注/取关。
func (a *App) Follow(ctx context.Context, userId, followUserId int64, isFollow bool) dto.Result {
	key := fmt.Sprintf("%s%d", rds.FollowsKey, userId)
	member := strconv.FormatInt(followUserId, 10)
	if isFollow {
		if err := a.DB.WithContext(ctx).Exec(
			"INSERT INTO tb_follow (user_id, follow_user_id) VALUES (?, ?)", userId, followUserId).Error; err != nil {
			return dto.Fail("服务器异常")
		}
		a.RDB.SAdd(ctx, key, member)
	} else {
		if err := a.DB.WithContext(ctx).Exec(
			"DELETE FROM tb_follow WHERE user_id = ? AND follow_user_id = ?", userId, followUserId).Error; err != nil {
			return dto.Fail("服务器异常")
		}
		a.RDB.SRem(ctx, key, member)
	}
	return dto.Ok()
}

// IsFollow 是否关注。
func (a *App) IsFollow(ctx context.Context, userId, targetId int64) dto.Result {
	var count int64
	if err := a.DB.WithContext(ctx).Model(&repo.Follow{}).
		Where("user_id = ? AND follow_user_id = ?", userId, targetId).Count(&count).Error; err != nil {
		return dto.Fail("服务器异常")
	}
	return dto.OkData(count > 0)
}

// FollowCommons 共同关注：SINTER + 用户列表。
func (a *App) FollowCommons(ctx context.Context, userId, targetId int64) dto.Result {
	k1 := fmt.Sprintf("%s%d", rds.FollowsKey, userId)
	k2 := fmt.Sprintf("%s%d", rds.FollowsKey, targetId)
	members, err := a.RDB.SInter(ctx, k1, k2).Result()
	if err != nil || len(members) == 0 {
		return dto.OkData([]dto.UserDTO{})
	}
	ids := make([]int64, 0, len(members))
	for _, m := range members {
		if id, e := strconv.ParseInt(m, 10, 64); e == nil {
			ids = append(ids, id)
		}
	}
	var users []repo.User
	if err := a.DB.WithContext(ctx).Where("id IN ?", ids).Find(&users).Error; err != nil {
		return dto.Fail("服务器异常")
	}
	out := make([]dto.UserDTO, 0, len(users))
	for i := range users {
		out = append(out, userToDTO(&users[i]))
	}
	return dto.OkData(out)
}

var _ = json.Marshal

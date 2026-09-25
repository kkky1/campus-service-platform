// Package repo GORM 模型（对应 hmdp.sql 的 10 张表）与自定义 SQL。
// 字段顺序对齐 Java 实体声明顺序，指针字段复刻 Jackson non_null 语义。
package repo

import "campus-service-platform/internal/pkg/dto"

// User tb_user
type User struct {
	Id         *int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id,omitempty"`
	Phone      *string    `gorm:"column:phone" json:"phone,omitempty"`
	Password   *string    `gorm:"column:password" json:"password,omitempty"`
	NickName   *string    `gorm:"column:nick_name" json:"nickName,omitempty"`
	Icon       *string    `gorm:"column:icon" json:"icon,omitempty"`
	CreateTime *dto.TimeT `gorm:"column:create_time" json:"createTime,omitempty"`
	UpdateTime *dto.TimeT `gorm:"column:update_time" json:"updateTime,omitempty"`
}

func (User) TableName() string { return "tb_user" }

// Shop tb_shop。缓存路径由 hutool 序列化，createTime/updateTime 用空格格式。
type Shop struct {
	Id         *int64         `gorm:"column:id;primaryKey;autoIncrement" json:"id,omitempty"`
	Name       *string        `gorm:"column:name" json:"name,omitempty"`
	TypeId     *int64         `gorm:"column:type_id" json:"typeId,omitempty"`
	Images     *string        `gorm:"column:images" json:"images,omitempty"`
	Area       *string        `gorm:"column:area" json:"area,omitempty"`
	Address    *string        `gorm:"column:address" json:"address,omitempty"`
	X          *float64       `gorm:"column:x" json:"x,omitempty"`
	Y          *float64       `gorm:"column:y" json:"y,omitempty"`
	AvgPrice   *int64         `gorm:"column:avg_price" json:"avgPrice,omitempty"`
	Sold       *int           `gorm:"column:sold" json:"sold,omitempty"`
	Comments   *int           `gorm:"column:comments" json:"comments,omitempty"`
	Score      *int           `gorm:"column:score" json:"score,omitempty"`
	OpenHours  *string        `gorm:"column:open_hours" json:"openHours,omitempty"`
	CreateTime *dto.TimeSpace `gorm:"column:create_time" json:"createTime,omitempty"`
	UpdateTime *dto.TimeSpace `gorm:"column:update_time" json:"updateTime,omitempty"`
	Distance   *float64       `gorm:"-" json:"distance,omitempty"`
}

func (Shop) TableName() string { return "tb_shop" }

// ShopType tb_shop_type。createTime/updateTime 为 @JsonIgnore，不参与 JSON。
type ShopType struct {
	Id         *int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id,omitempty"`
	Name       *string    `gorm:"column:name" json:"name,omitempty"`
	Icon       *string    `gorm:"column:icon" json:"icon,omitempty"`
	Sort       *int       `gorm:"column:sort" json:"sort,omitempty"`
	CreateTime *dto.TimeT `gorm:"column:create_time" json:"-"`
	UpdateTime *dto.TimeT `gorm:"column:update_time" json:"-"`
}

func (ShopType) TableName() string { return "tb_shop_type" }

// Voucher tb_voucher。stock/beginTime/endTime 来自 LEFT JOIN，非表列。
type Voucher struct {
	Id          *int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id,omitempty"`
	ShopId      *int64     `gorm:"column:shop_id" json:"shopId,omitempty"`
	Title       *string    `gorm:"column:title" json:"title,omitempty"`
	SubTitle    *string    `gorm:"column:sub_title" json:"subTitle,omitempty"`
	Rules       *string    `gorm:"column:rules" json:"rules,omitempty"`
	PayValue    *int64     `gorm:"column:pay_value" json:"payValue,omitempty"`
	ActualValue *int64     `gorm:"column:actual_value" json:"actualValue,omitempty"`
	Type        *int       `gorm:"column:type" json:"type,omitempty"`
	Status      *int       `gorm:"column:status" json:"status,omitempty"`
	Stock       *int       `gorm:"column:stock" json:"stock,omitempty"`
	BeginTime   *dto.TimeT `gorm:"column:begin_time" json:"beginTime,omitempty"`
	EndTime     *dto.TimeT `gorm:"column:end_time" json:"endTime,omitempty"`
	CreateTime  *dto.TimeT `gorm:"column:create_time" json:"createTime,omitempty"`
	UpdateTime  *dto.TimeT `gorm:"column:update_time" json:"updateTime,omitempty"`
}

func (Voucher) TableName() string { return "tb_voucher" }

// SeckillVoucher tb_seckill_voucher
type SeckillVoucher struct {
	VoucherId  *int64     `gorm:"column:voucher_id;primaryKey" json:"voucherId,omitempty"`
	Stock      *int       `gorm:"column:stock" json:"stock,omitempty"`
	CreateTime *dto.TimeT `gorm:"column:create_time" json:"createTime,omitempty"`
	BeginTime  *dto.TimeT `gorm:"column:begin_time" json:"beginTime,omitempty"`
	EndTime    *dto.TimeT `gorm:"column:end_time" json:"endTime,omitempty"`
	UpdateTime *dto.TimeT `gorm:"column:update_time" json:"updateTime,omitempty"`
}

func (SeckillVoucher) TableName() string { return "tb_seckill_voucher" }

// VoucherOrder tb_voucher_order（Kafka 消息仅含 id/userId/voucherId）
type VoucherOrder struct {
	Id         *int64     `gorm:"column:id;primaryKey" json:"id,omitempty"`
	UserId     *int64     `gorm:"column:user_id" json:"userId,omitempty"`
	VoucherId  *int64     `gorm:"column:voucher_id" json:"voucherId,omitempty"`
	PayType    *int       `gorm:"column:pay_type" json:"payType,omitempty"`
	Status     *int       `gorm:"column:status" json:"status,omitempty"`
	CreateTime *dto.TimeT `gorm:"column:create_time" json:"createTime,omitempty"`
	PayTime    *dto.TimeT `gorm:"column:pay_time" json:"payTime,omitempty"`
	UseTime    *dto.TimeT `gorm:"column:use_time" json:"useTime,omitempty"`
	RefundTime *dto.TimeT `gorm:"column:refund_time" json:"refundTime,omitempty"`
	UpdateTime *dto.TimeT `gorm:"column:update_time" json:"updateTime,omitempty"`
}

func (VoucherOrder) TableName() string { return "tb_voucher_order" }

// Blog tb_blog。icon/name/isLike 为非表列（运行时附加）。
type Blog struct {
	Id         *int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id,omitempty"`
	ShopId     *int64     `gorm:"column:shop_id" json:"shopId,omitempty"`
	UserId     *int64     `gorm:"column:user_id" json:"userId,omitempty"`
	Icon       *string    `gorm:"-" json:"icon,omitempty"`
	Name       *string    `gorm:"-" json:"name,omitempty"`
	IsLike     *bool      `gorm:"-" json:"isLike,omitempty"`
	Title      *string    `gorm:"column:title" json:"title,omitempty"`
	Images     *string    `gorm:"column:images" json:"images,omitempty"`
	Content    *string    `gorm:"column:content" json:"content,omitempty"`
	Liked      *int       `gorm:"column:liked" json:"liked,omitempty"`
	Comments   *int       `gorm:"column:comments" json:"comments,omitempty"`
	CreateTime *dto.TimeT `gorm:"column:create_time" json:"createTime,omitempty"`
	UpdateTime *dto.TimeT `gorm:"column:update_time" json:"updateTime,omitempty"`
}

func (Blog) TableName() string { return "tb_blog" }

// BlogComments tb_blog_comments（无接口，保留表模型）
type BlogComments struct {
	Id         *int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id,omitempty"`
	UserId     *int64     `gorm:"column:user_id" json:"userId,omitempty"`
	BlogId     *int64     `gorm:"column:blog_id" json:"blogId,omitempty"`
	ParentId   *int64     `gorm:"column:parent_id" json:"parentId,omitempty"`
	AnswerId   *int64     `gorm:"column:answer_id" json:"answerId,omitempty"`
	Content    *string    `gorm:"column:content" json:"content,omitempty"`
	Liked      *int       `gorm:"column:liked" json:"liked,omitempty"`
	Status     *bool      `gorm:"column:status" json:"status,omitempty"`
	CreateTime *dto.TimeT `gorm:"column:create_time" json:"createTime,omitempty"`
	UpdateTime *dto.TimeT `gorm:"column:update_time" json:"updateTime,omitempty"`
}

func (BlogComments) TableName() string { return "tb_blog_comments" }

// Follow tb_follow
type Follow struct {
	Id           *int64     `gorm:"column:id;primaryKey;autoIncrement" json:"id,omitempty"`
	UserId       *int64     `gorm:"column:user_id" json:"userId,omitempty"`
	FollowUserId *int64     `gorm:"column:follow_user_id" json:"followUserId,omitempty"`
	CreateTime   *dto.TimeT `gorm:"column:create_time" json:"createTime,omitempty"`
}

func (Follow) TableName() string { return "tb_follow" }

// UserInfo tb_user_info
type UserInfo struct {
	UserId     *int64        `gorm:"column:user_id;primaryKey" json:"userId,omitempty"`
	City       *string       `gorm:"column:city" json:"city,omitempty"`
	Introduce  *string       `gorm:"column:introduce" json:"introduce,omitempty"`
	Fans       *int          `gorm:"column:fans" json:"fans,omitempty"`
	Followee   *int          `gorm:"column:followee" json:"followee,omitempty"`
	Gender     *bool         `gorm:"column:gender" json:"gender,omitempty"`
	Birthday   *dto.DateOnly `gorm:"column:birthday" json:"birthday,omitempty"`
	Credits    *int          `gorm:"column:credits" json:"credits,omitempty"`
	Level      *bool         `gorm:"column:level" json:"level,omitempty"`
	CreateTime *dto.TimeT    `gorm:"column:create_time" json:"createTime,omitempty"`
	UpdateTime *dto.TimeT    `gorm:"column:update_time" json:"updateTime,omitempty"`
}

func (UserInfo) TableName() string { return "tb_user_info" }

package service

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"campus-service-platform/internal/pkg/dto"
	"campus-service-platform/internal/pkg/rds"
	"campus-service-platform/internal/repo"
	"campus-service-platform/scripts/lua"
)

// AddVoucher 券发布：/voucher 与 /voucher/seckill 行为一致（事务双表 + Redis 库存预载）。
func (a *App) AddVoucher(ctx context.Context, v *repo.Voucher) dto.Result {
	if v.Stock == nil {
		// 原系统：库存预载阶段 NPE → 全局异常，事务回滚
		return dto.Fail("服务器异常")
	}
	err := a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		cols := voucherColumns(v)
		if err := tx.Select(cols).Create(v).Error; err != nil {
			return err
		}
		sv := repo.SeckillVoucher{
			VoucherId: v.Id,
			Stock:     v.Stock,
			BeginTime: v.BeginTime,
			EndTime:   v.EndTime,
		}
		scols := []string{"voucher_id"}
		if sv.Stock != nil {
			scols = append(scols, "stock")
		}
		if sv.BeginTime != nil {
			scols = append(scols, "begin_time")
		}
		if sv.EndTime != nil {
			scols = append(scols, "end_time")
		}
		return tx.Select(scols).Create(&sv).Error
	})
	if err != nil {
		a.Log.Error("发布券失败", "err", err)
		return dto.Fail("服务器异常")
	}
	if err := a.RDB.Set(ctx, fmt.Sprintf("%s%d", rds.SeckillStockKey, *v.Id), fmt.Sprint(*v.Stock), 0).Err(); err != nil {
		return dto.Fail("服务器异常")
	}
	return dto.OkData(*v.Id)
}

func voucherColumns(v *repo.Voucher) []string {
	var cols []string
	if v.ShopId != nil {
		cols = append(cols, "shop_id")
	}
	if v.Title != nil {
		cols = append(cols, "title")
	}
	if v.SubTitle != nil {
		cols = append(cols, "sub_title")
	}
	if v.Rules != nil {
		cols = append(cols, "rules")
	}
	if v.PayValue != nil {
		cols = append(cols, "pay_value")
	}
	if v.ActualValue != nil {
		cols = append(cols, "actual_value")
	}
	if v.Type != nil {
		cols = append(cols, "type")
	}
	if v.Status != nil {
		cols = append(cols, "status")
	}
	return cols
}

// QueryVoucherOfShop 店铺券列表（LEFT JOIN 秒杀表）。
func (a *App) QueryVoucherOfShop(ctx context.Context, shopId int64) dto.Result {
	var list []repo.Voucher
	err := a.DB.WithContext(ctx).Raw(`
		SELECT v.id, v.shop_id, v.title, v.sub_title, v.rules, v.pay_value,
		       v.actual_value, v.type, sv.stock, sv.begin_time, sv.end_time
		FROM tb_voucher v
		LEFT JOIN tb_seckill_voucher sv ON v.id = sv.voucher_id
		WHERE v.shop_id = ? AND v.status = 1`, shopId).Scan(&list).Error
	if err != nil {
		return dto.Fail("服务器异常")
	}
	if list == nil {
		list = []repo.Voucher{}
	}
	return dto.OkData(list)
}

// NextOrderId 生成订单号：(now - 1640995200) << 32 | count。
func (a *App) NextOrderId(ctx context.Context) (int64, error) {
	now := time.Now()
	key := fmt.Sprintf("%s%s", rds.IcrOrderKey, now.Format("2006:01:02"))
	count, err := a.RDB.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	timestamp := now.Unix() - rds.BeginTimestamp
	return timestamp<<32 | (count & 0xFFFFFFFF), nil
}

// SeckillVoucher 抢券：Lua 原子校验 → Kafka 发布 → 立即返回订单号。
func (a *App) SeckillVoucher(ctx context.Context, userId, voucherId int64) dto.Result {
	orderId, err := a.NextOrderId(ctx)
	if err != nil {
		return dto.Fail("服务器异常")
	}
	res, err := a.RDB.Eval(ctx, lua.SeckillScript, nil,
		fmt.Sprint(voucherId), fmt.Sprint(userId), fmt.Sprint(orderId)).Int64()
	if err != nil {
		a.Log.Error("抢券 Lua 执行失败", "err", err)
		return dto.Fail("服务器异常")
	}
	switch res {
	case rds.SeckillNoStock:
		return dto.Fail("库存不足")
	case rds.SeckillRepeat:
		return dto.Fail("不能重复下单")
	}
	msg := SeckillOrderMsg{Id: orderId, UserId: userId, VoucherId: voucherId}
	if a.Pub == nil {
		return dto.Fail("服务器异常")
	}
	if err := a.Pub.PublishSeckillOrder(ctx, msg); err != nil {
		a.Log.Error("发送 Kafka 消息失败", "orderId", orderId, "err", err)
		return dto.Fail("服务器异常")
	}
	return dto.OkData(orderId)
}

// CreateOrderIdempotent 幂等建单：主键冲突不报错；仅真正插入时扣库存。
// 返回是否真正插入。
func CreateOrderIdempotent(ctx context.Context, db *gorm.DB, msg SeckillOrderMsg) (bool, error) {
	res := db.WithContext(ctx).Exec(
		"INSERT INTO tb_voucher_order (id, user_id, voucher_id) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE id = id",
		msg.Id, msg.UserId, msg.VoucherId)
	if res.Error != nil {
		return false, res.Error
	}
	inserted := res.RowsAffected == 1
	if !inserted {
		return false, nil
	}
	if err := db.WithContext(ctx).Exec(
		"UPDATE tb_seckill_voucher SET stock = stock - 1 WHERE voucher_id = ? AND stock > 0",
		msg.VoucherId).Error; err != nil {
		return true, err
	}
	return true, nil
}

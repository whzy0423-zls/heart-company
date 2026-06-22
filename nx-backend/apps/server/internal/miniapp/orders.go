// 订单与付费解锁：深度报告单次解锁的下单、支付成功落账、解锁查询。
package miniapp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"
)

type Order struct {
	ID            string `json:"id"`
	OutTradeNo    string `json:"outTradeNo"`
	Product       string `json:"product"`
	RefID         string `json:"refId"`
	Title         string `json:"title"`
	Amount        int    `json:"amount"`
	Status        string `json:"status"`
	TransactionID string `json:"transactionId"`
	CreateTime    string `json:"createTime"`
}

// CreateOrder 新建一个待支付订单。out_trade_no 由调用方生成（保证唯一）。
func (s *Store) CreateOrder(ctx context.Context, userID int64, outTradeNo, product string, refID int64, title string, amountCents int) (Order, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()

	var id int64
	var ct time.Time
	err := s.db.QueryRowContext(c,
		`INSERT INTO orders (out_trade_no, wx_user_id, product, ref_id, title, amount, status)
		 VALUES ($1,$2,$3,$4,$5,$6,'pending')
		 RETURNING id, create_time`,
		outTradeNo, userID, product, refID, title, amountCents,
	).Scan(&id, &ct)
	if err != nil {
		return Order{}, err
	}
	return Order{
		ID:         strconv.FormatInt(id, 10),
		OutTradeNo: outTradeNo,
		Product:    product,
		RefID:      strconv.FormatInt(refID, 10),
		Title:      title,
		Amount:     amountCents,
		Status:     "pending",
		CreateTime: fmtTime(ct),
	}, nil
}

// OrderByOutTradeNo 查订单（回调用）。
func (s *Store) OrderByOutTradeNo(ctx context.Context, outTradeNo string) (orderID, wxUserID, refID int64, product, status string, err error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	err = s.db.QueryRowContext(c,
		`SELECT id, wx_user_id, ref_id, product, status FROM orders WHERE out_trade_no=$1`, outTradeNo,
	).Scan(&orderID, &wxUserID, &refID, &product, &status)
	return
}

// MarkOrderPaid 支付成功落账：幂等地把订单置为 paid，并按产品类型发放权益。
// report 产品 → 写 report_unlocks；member 产品 → 抬升 wx_users.member_level。
// 返回是否为本次新置（true 表示首次确认，可用于决定是否发通知）。
func (s *Store) MarkOrderPaid(ctx context.Context, outTradeNo, transactionID string) (bool, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()

	tx, err := s.db.BeginTx(c, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	var orderID, wxUserID, refID int64
	var product, status string
	err = tx.QueryRowContext(c,
		`SELECT id, wx_user_id, ref_id, product, status FROM orders WHERE out_trade_no=$1 FOR UPDATE`, outTradeNo,
	).Scan(&orderID, &wxUserID, &refID, &product, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("order not found: %s", outTradeNo)
	}
	if err != nil {
		return false, err
	}
	if status == "paid" {
		return false, nil // 已处理过，幂等返回
	}

	if _, err := tx.ExecContext(c,
		`UPDATE orders SET status='paid', transaction_id=$1, paid_at=now(), update_time=now() WHERE id=$2`,
		transactionID, orderID,
	); err != nil {
		return false, err
	}

	switch product {
	case "report":
		if refID > 0 {
			if _, err := tx.ExecContext(c,
				`INSERT INTO report_unlocks (wx_user_id, test_record_id, order_id)
				 VALUES ($1,$2,$3) ON CONFLICT (wx_user_id, test_record_id) DO NOTHING`,
				wxUserID, refID, orderID,
			); err != nil {
				return false, err
			}
		}
	case "member":
		if _, err := tx.ExecContext(c,
			`UPDATE wx_users SET member_level=GREATEST(member_level,1) WHERE id=$1`, wxUserID,
		); err != nil {
			return false, err
		}
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// IsReportUnlocked 查询某用户对某测试记录是否已解锁深度报告。
func (s *Store) IsReportUnlocked(ctx context.Context, userID, testRecordID int64) (bool, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	var exists bool
	err := s.db.QueryRowContext(c,
		`SELECT EXISTS(SELECT 1 FROM report_unlocks WHERE wx_user_id=$1 AND test_record_id=$2)`,
		userID, testRecordID,
	).Scan(&exists)
	return exists, err
}

// TestRecordOwner 校验测试记录归属（防止替别人下单解锁）。
func (s *Store) TestRecordOwner(ctx context.Context, testRecordID int64) (int64, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	var owner int64
	err := s.db.QueryRowContext(c,
		`SELECT wx_user_id FROM test_records WHERE id=$1`, testRecordID,
	).Scan(&owner)
	return owner, err
}

// OpenIDByUserID 取微信 openid（下单时作为 payer）。
func (s *Store) OpenIDByUserID(ctx context.Context, userID int64) (string, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	var openid string
	err := s.db.QueryRowContext(c, `SELECT openid FROM wx_users WHERE id=$1`, userID).Scan(&openid)
	return openid, err
}

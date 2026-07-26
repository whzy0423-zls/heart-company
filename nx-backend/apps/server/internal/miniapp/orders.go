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

type PaymentOrderSnapshot struct {
	Amount     int
	OutTradeNo string
	Product    string
	Status     string
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

// CreateOrReusePendingOrder 幂等创建待支付订单。
// 同一用户/产品/关联对象已有金额和标题一致的 pending 订单时复用原 out_trade_no，避免重复点击产生多笔待支付订单。
// 如果价格或标题已变化，关闭旧 pending 单并创建新单，避免预支付金额和订单快照不一致导致回调验单失败。
func (s *Store) CreateOrReusePendingOrder(ctx context.Context, userID int64, outTradeNo, product string, refID int64, title string, amountCents int) (Order, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()

	tx, err := s.db.BeginTx(c, nil)
	if err != nil {
		return Order{}, err
	}
	defer func() { _ = tx.Rollback() }()

	lockKey := fmt.Sprintf("order:%d:%s:%d", userID, product, refID)
	if _, err := tx.ExecContext(c, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return Order{}, err
	}

	order, err := queryPendingOrder(c, tx, userID, product, refID)
	if err == nil {
		if order.Amount == amountCents && order.Title == title {
			if err := tx.Commit(); err != nil {
				return Order{}, err
			}
			return order, nil
		}
		if err := closePendingOrder(c, tx, order.ID); err != nil {
			return Order{}, err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Order{}, err
	}

	var id int64
	var ct time.Time
	err = tx.QueryRowContext(c,
		`INSERT INTO orders (out_trade_no, wx_user_id, product, ref_id, title, amount, status)
		 VALUES ($1,$2,$3,$4,$5,$6,'pending')
		 RETURNING id, create_time`,
		outTradeNo, userID, product, refID, title, amountCents,
	).Scan(&id, &ct)
	if err != nil {
		return Order{}, err
	}
	if err := tx.Commit(); err != nil {
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

type pendingOrderQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type pendingOrderExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func queryPendingOrder(ctx context.Context, q pendingOrderQuerier, userID int64, product string, refID int64) (Order, error) {
	var order Order
	var id int64
	var createTime time.Time
	err := q.QueryRowContext(ctx,
		`SELECT id, out_trade_no, product, ref_id::text, title, amount, status, transaction_id, create_time
		   FROM orders
		  WHERE wx_user_id=$1 AND product=$2 AND ref_id=$3 AND status='pending'
		  ORDER BY create_time DESC, id DESC
		  LIMIT 1
		  FOR UPDATE`,
		userID, product, refID,
	).Scan(&id, &order.OutTradeNo, &order.Product, &order.RefID, &order.Title, &order.Amount, &order.Status, &order.TransactionID, &createTime)
	if err != nil {
		return Order{}, err
	}
	order.ID = strconv.FormatInt(id, 10)
	order.CreateTime = fmtTime(createTime)
	return order, nil
}

func closePendingOrder(ctx context.Context, q pendingOrderExecer, orderID string) error {
	id, err := strconv.ParseInt(orderID, 10, 64)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx,
		`UPDATE orders SET status='closed', update_time=now() WHERE id=$1 AND status='pending'`,
		id,
	)
	return err
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

func (s *Store) PaymentOrderSnapshot(ctx context.Context, outTradeNo string) (PaymentOrderSnapshot, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	var order PaymentOrderSnapshot
	err := s.db.QueryRowContext(c,
		`SELECT out_trade_no, product, amount, status FROM orders WHERE out_trade_no=$1`, outTradeNo,
	).Scan(&order.OutTradeNo, &order.Product, &order.Amount, &order.Status)
	return order, err
}

// MarkOrderPaid 支付成功落账：幂等地把订单置为 paid，并按产品类型发放权益。
// report 产品 → 写 report_unlocks；member 产品 → 原子更新会员等级与有效期。
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
		// ref_id stores the purchased membership duration in days. Requiring an
		// explicit positive duration prevents new periodic memberships from
		// accidentally becoming lifetime memberships.
		if refID <= 0 {
			return false, fmt.Errorf("membership order missing duration")
		}
		if _, err := tx.ExecContext(c,
			`UPDATE wx_users
			 SET member_level=GREATEST(member_level,1),
			     member_started_at=CASE
			       WHEN member_level > 0 AND (member_expires_at IS NULL OR member_expires_at > now())
			         THEN COALESCE(member_started_at, now())
			       ELSE now()
			     END,
			     member_expires_at=CASE
			       WHEN member_level > 0 AND member_expires_at IS NULL THEN NULL
			       ELSE GREATEST(COALESCE(member_expires_at, now()), now()) + ($2 * interval '1 day')
			     END
			 WHERE id=$1`, wxUserID, refID,
		); err != nil {
			return false, err
		}
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// RevokeMembership immediately removes miniapp membership (for a refund or
// manual revocation). Classroom entitlement checks use the same wx_users
// fields, so no second membership source can remain active.
func (s *Store) RevokeMembership(ctx context.Context, userID int64) error {
	c, cancel := s.ctx(ctx)
	defer cancel()
	if userID <= 0 {
		return fmt.Errorf("invalid miniapp user id")
	}
	_, err := s.db.ExecContext(c,
		`UPDATE wx_users
		 SET member_level=0, member_started_at=NULL, member_expires_at=NULL
		 WHERE id=$1`, userID)
	return err
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

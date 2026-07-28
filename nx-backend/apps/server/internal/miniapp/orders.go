// 订单与付费解锁：深度报告单次解锁的下单、支付成功落账、解锁查询。
package miniapp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
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

const (
	ProductReport           = "report"
	ProductMember           = "member"
	ProductClassroomSeries  = "classroom_series"
	ProductClassroomContent = "classroom_content"
)

var (
	ErrOrderNotPayable             = errors.New("miniapp: order is not payable")
	ErrMembershipUserNotFound      = errors.New("miniapp: membership user not found")
	ErrPendingOrderSnapshotChanged = errors.New("miniapp: pending order snapshot changed")
	ErrOrderAlreadyOwned           = errors.New("miniapp: order target already owned")
)

type PaymentApplyResult struct {
	Changed        bool
	LateSuccess    bool
	PendingToClose []string
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

	lockKey := orderTargetLockKey(userID, product, refID)
	if _, err := tx.ExecContext(c, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return Order{}, err
	}
	if isClassroomProduct(product) {
		owned, err := classroomTargetOwned(c, tx, userID, product, refID)
		if err != nil {
			return Order{}, err
		}
		if owned {
			return Order{}, ErrOrderAlreadyOwned
		}
	}

	order, err := queryPendingOrder(c, tx, userID, product, refID)
	if err == nil {
		if order.Amount == amountCents && order.Title == title {
			if err := tx.Commit(); err != nil {
				return Order{}, err
			}
			return order, nil
		}
		return order, ErrPendingOrderSnapshotChanged
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

// ReplacePendingOrder is called only after the corresponding remote WeChat
// order has been closed. It rechecks ownership while holding the same target
// lock used by payment callbacks.
func (s *Store) ReplacePendingOrder(ctx context.Context, userID int64, outTradeNo, product string, refID int64, title string, amountCents int, oldOutTradeNo string) (Order, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	tx, err := s.db.BeginTx(c, nil)
	if err != nil {
		return Order{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(c, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, orderTargetLockKey(userID, product, refID)); err != nil {
		return Order{}, err
	}
	if isClassroomProduct(product) {
		owned, ownedErr := classroomTargetOwned(c, tx, userID, product, refID)
		if ownedErr != nil {
			return Order{}, ownedErr
		}
		if owned {
			return Order{}, ErrOrderAlreadyOwned
		}
	}
	var oldID, oldUserID, oldRefID int64
	var oldProduct, oldStatus string
	if err = tx.QueryRowContext(c, `SELECT id,wx_user_id,ref_id,product,status FROM orders WHERE out_trade_no=$1 FOR UPDATE`, oldOutTradeNo).
		Scan(&oldID, &oldUserID, &oldRefID, &oldProduct, &oldStatus); err != nil {
		return Order{}, err
	}
	if oldUserID != userID || oldRefID != refID || oldProduct != product {
		return Order{}, errors.New("pending order target mismatch")
	}
	if oldStatus == "paid" {
		return Order{}, ErrOrderAlreadyOwned
	}
	if oldStatus != "pending" {
		return Order{}, fmt.Errorf("%w: status=%s", ErrOrderNotPayable, oldStatus)
	}
	if _, err = tx.ExecContext(c, `UPDATE orders SET status='closed',update_time=now() WHERE id=$1 AND status='pending'`, oldID); err != nil {
		return Order{}, err
	}
	var id int64
	var createdAt time.Time
	if err = tx.QueryRowContext(c, `INSERT INTO orders (out_trade_no,wx_user_id,product,ref_id,title,amount,status)
		VALUES ($1,$2,$3,$4,$5,$6,'pending') RETURNING id,create_time`, outTradeNo, userID, product, refID, title, amountCents).
		Scan(&id, &createdAt); err != nil {
		return Order{}, err
	}
	if err = tx.Commit(); err != nil {
		return Order{}, err
	}
	return Order{ID: strconv.FormatInt(id, 10), OutTradeNo: outTradeNo, Product: product, RefID: strconv.FormatInt(refID, 10), Title: title, Amount: amountCents, Status: "pending", CreateTime: fmtTime(createdAt)}, nil
}

type pendingOrderQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
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
	result, err := s.MarkOrderPaidDetailed(ctx, outTradeNo, transactionID)
	return result.Changed, err
}

// MarkOrderPaidDetailed serializes payment success with order creation by the
// immutable user/product/ref target. A real SUCCESS is authoritative even when
// a local close won the earlier race: the order is restored to paid, access is
// issued, and competing pending orders are returned for remote close.
func (s *Store) MarkOrderPaidDetailed(ctx context.Context, outTradeNo, transactionID string) (PaymentApplyResult, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()

	var targetUserID, targetRefID int64
	var targetProduct string
	if err := s.db.QueryRowContext(c, `SELECT wx_user_id,ref_id,product FROM orders WHERE out_trade_no=$1`, outTradeNo).
		Scan(&targetUserID, &targetRefID, &targetProduct); err != nil {
		return PaymentApplyResult{}, err
	}

	tx, err := s.db.BeginTx(c, nil)
	if err != nil {
		return PaymentApplyResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(c, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, orderTargetLockKey(targetUserID, targetProduct, targetRefID)); err != nil {
		return PaymentApplyResult{}, err
	}

	var orderID, wxUserID, refID int64
	var product, status string
	err = tx.QueryRowContext(c,
		`SELECT id, wx_user_id, ref_id, product, status FROM orders WHERE out_trade_no=$1 FOR UPDATE`, outTradeNo,
	).Scan(&orderID, &wxUserID, &refID, &product, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return PaymentApplyResult{}, fmt.Errorf("order not found: %s", outTradeNo)
	}
	if err != nil {
		return PaymentApplyResult{}, err
	}
	if wxUserID != targetUserID || refID != targetRefID || product != targetProduct {
		return PaymentApplyResult{}, errors.New("order target changed while applying payment")
	}
	if status == "paid" {
		pending, closeErr := closeSiblingPendingOrders(c, tx, wxUserID, product, refID, orderID)
		if closeErr != nil {
			return PaymentApplyResult{}, closeErr
		}
		if err = tx.Commit(); err != nil {
			return PaymentApplyResult{}, err
		}
		return PaymentApplyResult{PendingToClose: pending}, nil
	}
	lateSuccess := status == "closed" && (product == ProductReport || isClassroomProduct(product))
	if status != "pending" && !lateSuccess {
		return PaymentApplyResult{}, fmt.Errorf("%w: status=%s", ErrOrderNotPayable, status)
	}

	if _, err := tx.ExecContext(c,
		`UPDATE orders SET status='paid', transaction_id=$1, paid_at=now(), update_time=now() WHERE id=$2`,
		transactionID, orderID,
	); err != nil {
		return PaymentApplyResult{}, err
	}

	switch product {
	case ProductReport:
		if refID > 0 {
			if _, err := tx.ExecContext(c,
				`INSERT INTO report_unlocks (wx_user_id, test_record_id, order_id)
				 VALUES ($1,$2,$3) ON CONFLICT (wx_user_id, test_record_id) DO NOTHING`,
				wxUserID, refID, orderID,
			); err != nil {
				return PaymentApplyResult{}, err
			}
		}
	case ProductMember:
		if refID <= 0 {
			return PaymentApplyResult{}, errors.New("membership order missing duration")
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
			return PaymentApplyResult{}, err
		}
	case ProductClassroomSeries:
		if refID <= 0 {
			return PaymentApplyResult{}, errors.New("classroom series order missing target")
		}
		if _, err := tx.ExecContext(c,
			`INSERT INTO classroom_entitlements (wx_user_id,series_id,order_id,source)
			 VALUES ($1,$2,$3,'purchase') ON CONFLICT (order_id) WHERE order_id IS NOT NULL DO NOTHING`,
			wxUserID, refID, orderID,
		); err != nil {
			return PaymentApplyResult{}, err
		}
	case ProductClassroomContent:
		if refID <= 0 {
			return PaymentApplyResult{}, errors.New("classroom content order missing target")
		}
		if _, err := tx.ExecContext(c,
			`INSERT INTO classroom_entitlements (wx_user_id,content_id,order_id,source)
			 VALUES ($1,$2,$3,'purchase') ON CONFLICT (order_id) WHERE order_id IS NOT NULL DO NOTHING`,
			wxUserID, refID, orderID,
		); err != nil {
			return PaymentApplyResult{}, err
		}
	default:
		return PaymentApplyResult{}, fmt.Errorf("unsupported payment product: %s", product)
	}

	pending, err := closeSiblingPendingOrders(c, tx, wxUserID, product, refID, orderID)
	if err != nil {
		return PaymentApplyResult{}, err
	}
	if lateSuccess {
		if _, err = tx.ExecContext(c, `INSERT INTO admin_operation_logs
			(operator_id,operator_name,action,target_type,target_id,before_data,after_data,summary)
			VALUES (0,'system','payment_late_success','order',$1,$2::jsonb,$3::jsonb,'late payment success compensated')`,
			strconv.FormatInt(orderID, 10), fmt.Sprintf(`{"status":"closed","outTradeNo":%q}`, outTradeNo), fmt.Sprintf(`{"status":"paid","transactionId":%q}`, transactionID)); err != nil {
			return PaymentApplyResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return PaymentApplyResult{}, err
	}
	return PaymentApplyResult{Changed: true, LateSuccess: lateSuccess, PendingToClose: pending}, nil
}

func closeSiblingPendingOrders(ctx context.Context, tx *sql.Tx, userID int64, product string, refID, paidOrderID int64) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT out_trade_no FROM orders
		WHERE wx_user_id=$1 AND product=$2 AND ref_id=$3 AND status='pending' AND id<>$4 FOR UPDATE`, userID, product, refID, paidOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []string{}
	for rows.Next() {
		var value string
		if err = rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

// ClosePendingOrders finalizes local state only after all corresponding
// remote WeChat orders were confirmed closed. Until then repeated payment
// callbacks continue returning the same retry set.
func (s *Store) ClosePendingOrders(ctx context.Context, outTradeNos []string) error {
	if len(outTradeNos) == 0 {
		return nil
	}
	c, cancel := s.ctx(ctx)
	defer cancel()
	placeholders := make([]string, len(outTradeNos))
	args := make([]any, len(outTradeNos))
	for i, value := range outTradeNos {
		placeholders[i] = "$" + strconv.Itoa(i+1)
		args[i] = value
	}
	_, err := s.db.ExecContext(c, `UPDATE orders SET status='closed',update_time=now() WHERE status='pending' AND out_trade_no IN (`+strings.Join(placeholders, ",")+`)`, args...)
	return err
}

func orderTargetLockKey(userID int64, product string, refID int64) string {
	return fmt.Sprintf("order:%d:%s:%d", userID, product, refID)
}

func isClassroomProduct(product string) bool {
	return product == ProductClassroomSeries || product == ProductClassroomContent
}

func classroomTargetOwned(ctx context.Context, q pendingOrderQuerier, userID int64, product string, refID int64) (bool, error) {
	targetClause := "e.series_id=$2"
	if product == ProductClassroomContent {
		targetClause = "(e.content_id=$2 OR e.series_id=(SELECT series_id FROM classroom_contents WHERE id=$2))"
	}
	var owned bool
	err := q.QueryRowContext(ctx, fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM classroom_entitlements e WHERE e.wx_user_id=$1 AND %s AND e.revoked_at IS NULL AND (e.expires_at IS NULL OR e.expires_at>now()))`, targetClause), userID, refID).Scan(&owned)
	return owned, err
}

// LatestOrderForTarget returns the most recent immutable order snapshot for a
// classroom target owned by the user.
func (s *Store) LatestOrderForTarget(ctx context.Context, userID int64, product string, refID int64) (Order, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	var order Order
	var id int64
	var createTime time.Time
	err := s.db.QueryRowContext(c,
		`SELECT id,out_trade_no,product,ref_id::text,title,amount,status,transaction_id,create_time
		 FROM orders WHERE wx_user_id=$1 AND product=$2 AND ref_id=$3
		 ORDER BY create_time DESC,id DESC LIMIT 1`, userID, product, refID,
	).Scan(&id, &order.OutTradeNo, &order.Product, &order.RefID, &order.Title, &order.Amount, &order.Status, &order.TransactionID, &createTime)
	if err != nil {
		return Order{}, err
	}
	order.ID = strconv.FormatInt(id, 10)
	order.CreateTime = fmtTime(createTime)
	return order, nil
}

// RefundClassroomOrder atomically marks one paid classroom order refunded,
// revokes only the entitlement issued by that order, and writes an audit row.
func (s *Store) RefundClassroomOrder(ctx context.Context, outTradeNo, reason string) (bool, error) {
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
		`SELECT id,wx_user_id,ref_id,product,status FROM orders WHERE out_trade_no=$1 FOR UPDATE`, outTradeNo,
	).Scan(&orderID, &wxUserID, &refID, &product, &status)
	if err != nil {
		return false, err
	}
	if product != ProductClassroomSeries && product != ProductClassroomContent {
		return false, fmt.Errorf("not a classroom order: %s", product)
	}
	if status == "refunded" {
		return false, nil
	}
	if status != "paid" {
		return false, fmt.Errorf("%w: status=%s", ErrOrderNotPayable, status)
	}
	if _, err = tx.ExecContext(c, `UPDATE orders SET status='refunded',update_time=now() WHERE id=$1 AND status='paid'`, orderID); err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(c, `UPDATE classroom_entitlements SET revoked_at=COALESCE(revoked_at,now()),updated_at=now() WHERE order_id=$1`, orderID); err != nil {
		return false, err
	}
	summary := strings.TrimSpace(reason)
	if summary == "" {
		summary = "classroom order refunded"
	}
	if _, err = tx.ExecContext(c, `INSERT INTO admin_operation_logs
		(operator_id,operator_name,action,target_type,target_id,before_data,after_data,summary)
		VALUES (0,'system','classroom_entitlement_refund','order',$1,$2::jsonb,$3::jsonb,$4)`,
		strconv.FormatInt(orderID, 10),
		fmt.Sprintf(`{"status":%q,"product":%q,"refId":%d,"wxUserId":%d}`, status, product, refID, wxUserID),
		fmt.Sprintf(`{"status":"refunded","entitlementRevoked":true}`), summary,
	); err != nil {
		return false, err
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// RevokeAllMembership is an administrator-level full entitlement revocation.
// It deliberately clears every membership period for the user. It is not an
// order refund operation; order-scoped refund and audit handling must account
// for other paid periods before calling or superseding this boundary.
func (s *Store) RevokeAllMembership(ctx context.Context, userID int64) error {
	c, cancel := s.ctx(ctx)
	defer cancel()
	if userID <= 0 {
		return fmt.Errorf("invalid miniapp user id")
	}
	result, err := s.db.ExecContext(c,
		`UPDATE wx_users
		 SET member_level=0, member_started_at=NULL, member_expires_at=NULL
		 WHERE id=$1`, userID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrMembershipUserNotFound
	}
	return nil
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

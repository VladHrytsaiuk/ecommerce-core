package postgres

import (
	"time"

	"context"

	"github.com/google/uuid"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/order/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/db"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/shared/pagination"
	"gorm.io/gorm"
)

type orderRepository struct {
	db *gorm.DB
	l  logger.Logger
}

// NewOrderRepository створює новий інстанс репозиторію замовлень
func NewOrderRepository(db *gorm.DB, l logger.Logger) domain.OrderRepository {
	return &orderRepository{db: db, l: l}
}

// getDB повертає транзакцію з контексту, якщо вона є, інакше звичайне підключення.
func (r *orderRepository) getDB(ctx context.Context) *gorm.DB {
	return db.GetTx(ctx, r.db).WithContext(ctx)
}

// Create атомарно створює замовлення, елементи та доставку в одній транзакції
func (r *orderRepository) Create(ctx context.Context, order *domain.Order, items []domain.OrderItem, delivery *domain.Delivery) error {
	return r.getDB(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Створюємо замовлення (order_number генерується sequence в БД)
		if err := tx.Create(order).Error; err != nil {
			r.l.Errorw("failed to create order", "error", err)
			return err
		}

		// 2. Прив'язуємо items до order
		for i := range items {
			items[i].OrderID = order.ID
		}
		if err := tx.Create(&items).Error; err != nil {
			r.l.Errorw("failed to create order items", "error", err)
			return err
		}

		// 3. Створюємо запис доставки
		delivery.OrderID = order.ID
		if err := tx.Create(delivery).Error; err != nil {
			r.l.Errorw("failed to create delivery", "error", err)
			return err
		}

		// 4. Перечитуємо order, щоб отримати згенерований order_number
		if err := tx.First(order, "id = ?", order.ID).Error; err != nil {
			r.l.Errorw("failed to reload order after creation", "error", err)
			return err
		}

		return nil
	})
}

// FindByID знаходить замовлення за UUID з усіма зв'язками
func (r *orderRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	var order domain.Order
	err := r.getDB(ctx).
		Preload("Status").
		Preload("Items").
		Preload("Items.Variation.Product.Images").
		Preload("Items.Variation.Product.Translations").
		Preload("Delivery").
		First(&order, "id = ?", id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, domain.ErrOrderNotFound
		}
		r.l.Errorw("failed to find order by id", "error", err, "id", id)
		return nil, err
	}
	return &order, nil
}

// GetOrderStatusByID повертає лише status_id замовлення (легкий запит)
func (r *orderRepository) GetOrderStatusByID(ctx context.Context, id uuid.UUID) (int, error) {
	var statusID int
	err := r.getDB(ctx).
		Model(&domain.Order{}).
		Select("status_id").
		Where("id = ?", id).
		Scan(&statusID).Error
	if err != nil {
		r.l.Errorw("failed to find order status by id", "error", err, "id", id)
		return 0, err
	}
	if statusID == 0 {
		return 0, domain.ErrOrderNotFound
	}
	return statusID, nil
}

// FindByOrderNumber знаходить замовлення за номером
func (r *orderRepository) FindByOrderNumber(ctx context.Context, orderNumber int64) (*domain.Order, error) {
	var order domain.Order
	err := r.getDB(ctx).
		Preload("Status").
		Preload("Items").
		Preload("Items.Variation.Product.Images").
		Preload("Items.Variation.Product.Translations").
		Preload("Delivery").
		First(&order, "order_number = ?", orderNumber).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, domain.ErrOrderNotFound
		}
		r.l.Errorw("failed to find order by number", "error", err, "order_number", orderNumber)
		return nil, err
	}
	return &order, nil
}

// GetForUpdate знаходить замовлення за номером і блокує його (SELECT ... FOR UPDATE)
func (r *orderRepository) GetForUpdate(ctx context.Context, orderNumber int64) (*domain.Order, error) {
	var order domain.Order
	err := r.getDB(ctx).
		Preload("Status").
		Preload("Items").
		Preload("Items.Variation.Product.Images").
		Preload("Items.Variation.Product.Translations").
		Preload("Delivery").
		Set("gorm:query_option", "FOR UPDATE").
		First(&order, "order_number = ?", orderNumber).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, domain.ErrOrderNotFound
		}
		r.l.Errorw("failed to lock order by number", "error", err, "order_number", orderNumber)
		return nil, err
	}
	return &order, nil
}

// WithTransaction виконує функцію в межах транзакції.
// Зберігає tx у контексті через db.TxKey, щоб інші репозиторії могли його підхопити.
func (r *orderRepository) WithTransaction(ctx context.Context, fn func(ctx context.Context, txRepo domain.OrderRepository) error) error {
	return r.getDB(ctx).Transaction(func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, db.TxKey{}, tx)
		txRepo := &orderRepository{db: tx, l: r.l}
		return fn(txCtx, txRepo)
	})
}

// FindByUserID знаходить замовлення користувача з пагінацією
func (r *orderRepository) FindByUserID(ctx context.Context, userID uuid.UUID, pgn pagination.Params) ([]domain.Order, int64, error) {
	var orders []domain.Order
	var total int64

	baseQuery := r.getDB(ctx).
		Model(&domain.Order{}).
		Where("user_id = ?", userID)

	if err := baseQuery.Count(&total).Error; err != nil {
		r.l.Errorw("failed to count orders", "error", err, "user_id", userID)
		return nil, 0, err
	}

	err := r.getDB(ctx).
		Preload("Status").
		Preload("Items").
		Preload("Items.Variation.Product.Images").
		Preload("Items.Variation.Product.Translations").
		Preload("Delivery").
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Offset(pgn.GetOffset()).
		Limit(pgn.Limit).
		Find(&orders).Error
	if err != nil {
		r.l.Errorw("failed to find orders by user_id", "error", err, "user_id", userID)
		return nil, 0, err
	}

	return orders, total, nil
}

// UpdateStatus оновлює статус замовлення
func (r *orderRepository) UpdateStatus(ctx context.Context, orderID uuid.UUID, statusID int) error {
	result := r.getDB(ctx).
		Model(&domain.Order{}).
		Where("id = ?", orderID).
		Updates(map[string]interface{}{
			"status_id":  statusID,
			"updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
		})
	if result.Error != nil {
		r.l.Errorw("failed to update order status", "error", result.Error, "order_id", orderID, "status_id", statusID)
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrOrderNotFound
	}
	return nil
}

// Update оновлює замовлення
func (r *orderRepository) Update(ctx context.Context, order *domain.Order) error {
	if err := r.getDB(ctx).Save(order).Error; err != nil {
		r.l.Errorw("failed to update order", "error", err, "order_id", order.ID)
		return err
	}
	return nil
}

// SetManagerToken зберігає хешований менеджерський токен та час закінчення дії
func (r *orderRepository) SetManagerToken(ctx context.Context, orderID uuid.UUID, hash string, expiresAt time.Time) error {
	result := r.getDB(ctx).
		Model(&domain.Order{}).
		Where("id = ?", orderID).
		Updates(map[string]interface{}{
			"manager_token_hash":       hash,
			"manager_token_expires_at": expiresAt,
			"updated_at":               gorm.Expr("CURRENT_TIMESTAMP"),
		})
	if result.Error != nil {
		r.l.Errorw("failed to set manager token", "error", result.Error, "order_id", orderID)
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrOrderNotFound
	}
	return nil
}

// SetTTNData зберігає дані ТТН та carrier-відповідь.
// НЕ змінює status_id — це відповідальність сервісного рівня (ConfirmOrder).
func (r *orderRepository) SetTTNData(ctx context.Context, orderID uuid.UUID, ttnNumber, ttnRef, carrierStatus string, rawResponse *string) error {
	updates := map[string]interface{}{
		"ttn_number":     ttnNumber,
		"ttn_ref":        ttnRef,
		"ttn_created_at": gorm.Expr("CURRENT_TIMESTAMP"),
		"carrier_status": carrierStatus,
		"updated_at":     gorm.Expr("CURRENT_TIMESTAMP"),
	}
	if rawResponse != nil {
		updates["carrier_raw_response"] = *rawResponse
	}

	result := r.getDB(ctx).
		Model(&domain.Order{}).
		Where("id = ?", orderID).
		Updates(updates)
	if result.Error != nil {
		r.l.Errorw("failed to set TTN data", "error", result.Error, "order_id", orderID)
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrOrderNotFound
	}
	return nil
}

// GetOrdersForPaymentReminder знаходить замовлення, для яких потрібно відправити нагадування
func (r *orderRepository) GetOrdersForPaymentReminder(ctx context.Context, olderThan time.Time) ([]domain.Order, error) {
	var orders []domain.Order
	err := r.getDB(ctx).
		Preload("Items").
		Preload("Delivery").
		Where("status_id = ? AND created_at < ? AND payment_reminder_sent_at IS NULL", domain.StatusPendingPayment, olderThan).
		Find(&orders).Error
	if err != nil {
		r.l.Errorw("failed to get orders for payment reminder", "error", err)
		return nil, err
	}
	return orders, nil
}

// GetOrdersForPaymentTimeout знаходить замовлення, які потрібно скасувати через несплату
func (r *orderRepository) GetOrdersForPaymentTimeout(ctx context.Context, olderThan time.Time) ([]domain.Order, error) {
	var orders []domain.Order
	err := r.getDB(ctx).
		Where("status_id = ? AND created_at < ?", domain.StatusPendingPayment, olderThan).
		Find(&orders).Error
	if err != nil {
		r.l.Errorw("failed to get orders for payment timeout", "error", err)
		return nil, err
	}
	return orders, nil
}

// MarkPaymentReminderSent помічає, що нагадування було відправлено
func (r *orderRepository) MarkPaymentReminderSent(ctx context.Context, orderID uuid.UUID) error {
	result := r.getDB(ctx).
		Model(&domain.Order{}).
		Where("id = ?", orderID).
		Update("payment_reminder_sent_at", gorm.Expr("CURRENT_TIMESTAMP"))
	
	if result.Error != nil {
		r.l.Errorw("failed to mark payment reminder sent", "error", result.Error, "order_id", orderID)
		return result.Error
	}
	return nil
}

// FindByIDForUpdate знаходить замовлення за UUID і блокує його (SELECT ... FOR UPDATE)
func (r *orderRepository) FindByIDForUpdate(ctx context.Context, orderID uuid.UUID) (*domain.Order, error) {
	var order domain.Order
	err := r.getDB(ctx).
		Preload("Status").
		Preload("Items").
		Preload("Items.Variation.Product.Images").
		Preload("Items.Variation.Product.Translations").
		Preload("Delivery").
		Set("gorm:query_option", "FOR UPDATE").
		First(&order, "id = ?", orderID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, domain.ErrOrderNotFound
		}
		r.l.Errorw("failed to lock order by id", "error", err, "order_id", orderID)
		return nil, err
	}
	return &order, nil
}

// FindAllOrders знаходить замовлення з фільтрацією, пошуком та пагінацією (для адмінки)
func (r *orderRepository) FindAllOrders(ctx context.Context, filters domain.AdminOrderFilters) ([]domain.Order, int64, error) {
	var orders []domain.Order
	var total int64

	query := r.getDB(ctx).Model(&domain.Order{})

	// Фільтр за статусом
	if filters.StatusID != nil {
		query = query.Where("status_id = ?", *filters.StatusID)
	}

	// Пошук за №, email, телефоном, ім'ям
	if filters.Search != "" {
		search := "%" + filters.Search + "%"
		query = query.Where(
			`(CAST(order_number AS TEXT) LIKE ? OR email ILIKE ? OR phone ILIKE ? OR (first_name || ' ' || last_name) ILIKE ?)`,
			search, search, search, search,
		)
	}

	// Фільтр за датою
	if filters.DateFrom != nil {
		query = query.Where("created_at >= ?", *filters.DateFrom)
	}
	if filters.DateTo != nil {
		query = query.Where("created_at <= ?", *filters.DateTo)
	}

	// Count
	if err := query.Count(&total).Error; err != nil {
		r.l.Errorw("failed to count admin orders", "error", err)
		return nil, 0, err
	}

	// Сортування (дозволяємо лише безпечні поля)
	sortBy := "created_at"
	if filters.SortBy == "total_price" {
		sortBy = "total_price"
	}
	sortOrder := "DESC"
	if filters.Order == "asc" {
		sortOrder = "ASC"
	}
	orderClause := sortBy + " " + sortOrder

	err := query.
		Preload("Status").
		Preload("Items").
		Preload("Items.Variation.Product.Images").
		Preload("Items.Variation.Product.Translations").
		Preload("Delivery").
		Order(orderClause).
		Offset(filters.GetOffset()).
		Limit(filters.Limit).
		Find(&orders).Error
	if err != nil {
		r.l.Errorw("failed to find admin orders", "error", err)
		return nil, 0, err
	}

	return orders, total, nil
}

// UpdateAdminComment оновлює коментар адміністратора
func (r *orderRepository) UpdateAdminComment(ctx context.Context, orderID uuid.UUID, comment string) error {
	result := r.getDB(ctx).
		Model(&domain.Order{}).
		Where("id = ?", orderID).
		Updates(map[string]interface{}{
			"admin_comment": comment,
			"updated_at":    gorm.Expr("CURRENT_TIMESTAMP"),
		})
	if result.Error != nil {
		r.l.Errorw("failed to update admin comment", "error", result.Error, "order_id", orderID)
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrOrderNotFound
	}
	return nil
}

// UpdateCarrierData оновлює carrier_status та carrier_raw_response
func (r *orderRepository) UpdateCarrierData(ctx context.Context, orderID uuid.UUID, carrierStatus string, rawResponse *string) error {
	updates := map[string]interface{}{
		"carrier_status": carrierStatus,
		"updated_at":     gorm.Expr("CURRENT_TIMESTAMP"),
	}
	if rawResponse != nil {
		updates["carrier_raw_response"] = *rawResponse
	}

	result := r.getDB(ctx).
		Model(&domain.Order{}).
		Where("id = ?", orderID).
		Updates(updates)
	if result.Error != nil {
		r.l.Errorw("failed to update carrier data", "error", result.Error, "order_id", orderID)
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrOrderNotFound
	}
	return nil
}

// GetOrderStatuses повертає довідник статусів замовлень
func (r *orderRepository) GetOrderStatuses(ctx context.Context) ([]domain.OrderStatus, error) {
	var statuses []domain.OrderStatus
	err := r.getDB(ctx).Order("sort_order ASC").Find(&statuses).Error
	if err != nil {
		r.l.Errorw("failed to get order statuses", "error", err)
		return nil, err
	}
	return statuses, nil
}

// GetShippedOrders повертає замовлення зі статусом Shipped (для трекінгу)
func (r *orderRepository) GetShippedOrders(ctx context.Context) ([]domain.Order, error) {
	var orders []domain.Order
	err := r.getDB(ctx).
		Preload("Delivery").
		Where("status_id = ? AND ttn_number IS NOT NULL AND ttn_number <> ''", domain.StatusShipped).
		Find(&orders).Error
	if err != nil {
		r.l.Errorw("failed to get shipped orders for tracking", "error", err)
		return nil, err
	}
	return orders, nil
}

// CreateStatusHistory створює запис історії зміни статусу
func (r *orderRepository) CreateStatusHistory(ctx context.Context, history *domain.OrderStatusHistory) error {
	if err := r.getDB(ctx).Create(history).Error; err != nil {
		r.l.Errorw("failed to create status history", "error", err, "order_id", history.OrderID)
		return err
	}
	return nil
}

// FindStatusHistory повертає історію зміни статусів замовлення
func (r *orderRepository) FindStatusHistory(ctx context.Context, orderID uuid.UUID) ([]domain.OrderStatusHistory, error) {
	var history []domain.OrderStatusHistory
	err := r.getDB(ctx).
		Preload("FromStatus").
		Preload("ToStatus").
		Where("order_id = ?", orderID).
		Order("created_at ASC").
		Find(&history).Error
	if err != nil {
		r.l.Errorw("failed to find status history", "error", err, "order_id", orderID)
		return nil, err
	}
	return history, nil
}


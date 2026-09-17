package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	adminDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/admin/domain"
	coreEvents "github.com/VladHrytsaiuk/ecommerce-core/internal/core/events"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/core/money"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/http/apiresponse"
	returnsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/application"
	returns "github.com/VladHrytsaiuk/ecommerce-core/internal/returns/domain"
)

// A return moves money back to a customer, so the two properties worth pinning
// here are who the request is filed for and whose order it may be filed
// against. Both are decided below the handler, so these tests drive the real
// service and the real eligibility policy and fake only the ports.

func TestTheReturnIsFiledForTheAuthenticatedCustomer(t *testing.T) {
	world := newWorld(t)
	body := fmt.Sprintf(`{"order_id":%q,"customer_id":%q,"items":[{"variant_id":%q,"quantity":1,"condition":"unopened"}]}`,
		world.orderID, uuid.New(), world.variantID)

	recorder := world.post("/api/v1/customers/me/returns", body)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if world.repository.request == nil {
		t.Fatal("no return request was persisted")
	}
	// The body named a different customer. Ownership comes from the token, or
	// one customer could file refunds against another's account.
	if world.repository.request.CustomerID != world.customerID {
		t.Fatalf("return filed for %s, want the authenticated customer %s", world.repository.request.CustomerID, world.customerID)
	}
}

func TestAnAnonymousCallerCannotFileAReturn(t *testing.T) {
	world := newWorld(t)
	world.anonymous = true
	body := fmt.Sprintf(`{"order_id":%q,"items":[{"variant_id":%q,"quantity":1,"condition":"unopened"}]}`, world.orderID, world.variantID)

	recorder := world.post("/api/v1/customers/me/returns", body)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	if world.orders.calls != 0 || world.repository.request != nil {
		t.Fatal("an anonymous request reached the return service")
	}
}

func TestAReturnCannotBeFiledAgainstAnotherCustomersOrder(t *testing.T) {
	world := newWorld(t)
	stranger := uuid.New()
	world.orders.snapshot.CustomerID = &stranger
	body := fmt.Sprintf(`{"order_id":%q,"items":[{"variant_id":%q,"quantity":1,"condition":"unopened"}]}`, world.orderID, world.variantID)

	recorder := world.post("/api/v1/customers/me/returns", body)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	if world.repository.request != nil {
		t.Fatal("a return was persisted against an order belonging to someone else")
	}
}

func TestAnOrderWithNoOwnerIsNotReturnable(t *testing.T) {
	// A guest order has no customer to refund; it must not be claimable by
	// whoever happens to know the order ID.
	world := newWorld(t)
	world.orders.snapshot.CustomerID = nil
	body := fmt.Sprintf(`{"order_id":%q,"items":[{"variant_id":%q,"quantity":1,"condition":"unopened"}]}`, world.orderID, world.variantID)

	recorder := world.post("/api/v1/customers/me/returns", body)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	if world.repository.request != nil {
		t.Fatal("a return was persisted against an order with no owner")
	}
}

func TestMalformedReturnRequestsAreRejectedBeforeTheService(t *testing.T) {
	// The bounds keep a single request from expanding into thousands of RMA
	// rows or a 2000-column reason, so they have to hold at the edge.
	orderID, variantID := uuid.New(), uuid.New()
	item := func(fields string) string { return `{"variant_id":"` + variantID.String() + `",` + fields + `}` }
	bodies := map[string]string{
		"no order id":        `{"items":[` + item(`"quantity":1,"condition":"unopened"`) + `]}`,
		"nil order id":       `{"order_id":"00000000-0000-0000-0000-000000000000","items":[` + item(`"quantity":1,"condition":"unopened"`) + `]}`,
		"no items":           `{"order_id":"` + orderID.String() + `","items":[]}`,
		"items missing":      `{"order_id":"` + orderID.String() + `"}`,
		"too many items":     `{"order_id":"` + orderID.String() + `","items":[` + strings.Repeat(item(`"quantity":1,"condition":"unopened"`)+",", 50) + item(`"quantity":1,"condition":"unopened"`) + `]}`,
		"no variant id":      `{"order_id":"` + orderID.String() + `","items":[{"quantity":1,"condition":"unopened"}]}`,
		"zero quantity":      `{"order_id":"` + orderID.String() + `","items":[` + item(`"quantity":0,"condition":"unopened"`) + `]}`,
		"negative quantity":  `{"order_id":"` + orderID.String() + `","items":[` + item(`"quantity":-1,"condition":"unopened"`) + `]}`,
		"quantity too large": `{"order_id":"` + orderID.String() + `","items":[` + item(`"quantity":10001,"condition":"unopened"`) + `]}`,
		"no condition":       `{"order_id":"` + orderID.String() + `","items":[` + item(`"quantity":1`) + `]}`,
		"reason too long":    `{"order_id":"` + orderID.String() + `","items":[` + item(`"quantity":1,"condition":"unopened","reason":"`+strings.Repeat("a", 2001)+`"`) + `]}`,
		"not json":           `{`,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			world := newWorld(t)
			recorder := world.post("/api/v1/customers/me/returns", body)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
			if world.orders.calls != 0 {
				t.Fatal("a malformed request reached the return service")
			}
		})
	}
}

func TestARequestOutsideThePolicyIsAClientError(t *testing.T) {
	// Every one of these is the customer's request being refused, not the
	// server failing. A 5xx here would page someone and retry on the client.
	cases := map[string]func(*world){
		"window expired":        func(w *world) { w.orders.snapshot.DeliveredAt = ptr(w.now.AddDate(0, 0, -15)) },
		"order not delivered":   func(w *world) { w.orders.snapshot.Status = "processing" },
		"no eligibility date":   func(w *world) { w.orders.snapshot.DeliveredAt, w.orders.snapshot.PaidAt = nil, nil },
		"item not in the order": func(w *world) { w.orders.snapshot.Items = nil },
		"more than was bought":  func(w *world) { w.orders.snapshot.Items[0].Quantity = 1 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			world := newWorld(t)
			mutate(world)
			body := fmt.Sprintf(`{"order_id":%q,"items":[{"variant_id":%q,"quantity":2,"condition":"unopened"}]}`, world.orderID, world.variantID)

			recorder := world.post("/api/v1/customers/me/returns", body)

			if recorder.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusUnprocessableEntity, recorder.Body.String())
			}
			if world.repository.request != nil {
				t.Fatal("an ineligible return was persisted")
			}
		})
	}
}

func TestOnlyFullRefundsAreAccepted(t *testing.T) {
	// Partial and store-credit settlement have no amount policy yet; accepting
	// one would strand the RMA in the settlement DLQ.
	for _, mode := range []string{"partial", "store_credit"} {
		t.Run(mode, func(t *testing.T) {
			world := newWorld(t)
			body := fmt.Sprintf(`{"order_id":%q,"refund_mode":%q,"items":[{"variant_id":%q,"quantity":1,"condition":"unopened"}]}`, world.orderID, mode, world.variantID)

			recorder := world.post("/api/v1/customers/me/returns", body)

			if recorder.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusUnprocessableEntity, recorder.Body.String())
			}
			if world.repository.request != nil {
				t.Fatal("an unsupported refund mode was persisted")
			}
		})
	}
}

func TestTheCreatedReturnIsReportedWithoutInternalFields(t *testing.T) {
	world := newWorld(t)
	body := fmt.Sprintf(`{"order_id":%q,"items":[{"variant_id":%q,"quantity":1,"condition":"unopened","reason":"  too small  "}]}`, world.orderID, world.variantID)

	recorder := world.post("/api/v1/customers/me/returns", body)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		Data struct {
			ID         string `json:"id"`
			OrderID    string `json:"order_id"`
			Status     string `json:"status"`
			RefundMode string `json:"refund_mode"`
			Items      []struct {
				Quantity  int    `json:"quantity"`
				Condition string `json:"condition"`
				Reason    string `json:"reason"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.ID == "" || envelope.Data.OrderID != world.orderID.String() || envelope.Data.Status != "new" || envelope.Data.RefundMode != "full" {
		t.Fatalf("response = %+v", envelope.Data)
	}
	if len(envelope.Data.Items) != 1 || envelope.Data.Items[0].Reason != "too small" {
		t.Fatalf("items = %+v, want the reason trimmed", envelope.Data.Items)
	}
	if strings.Contains(recorder.Body.String(), "customer_id") {
		t.Fatal("the response echoes the customer ID back into the body")
	}
}

func TestAdminTransitionsRequireThePermission(t *testing.T) {
	for _, path := range []string{"approve", "receive"} {
		t.Run(path, func(t *testing.T) {
			world := newWorld(t)
			world.authorizer.err = adminDomain.ErrPermissionDenied
			world.seedReturn(returns.ReturnStatusNew)

			recorder := world.post("/api/v1/admin/returns/"+world.repository.request.ID.String()+"/"+path, `{}`)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusForbidden, recorder.Body.String())
			}
			if world.repository.request.Status != returns.ReturnStatusNew {
				t.Fatalf("status changed to %q without the permission", world.repository.request.Status)
			}
			if world.authorizer.permission != PermissionReturnsWrite {
				t.Fatalf("checked permission %q, want %q", world.authorizer.permission, PermissionReturnsWrite)
			}
		})
	}
}

func TestApprovalIsAppliedAndReceiptIsOnlyAccepted(t *testing.T) {
	world := newWorld(t)
	world.seedReturn(returns.ReturnStatusNew)
	id := world.repository.request.ID.String()

	// Approval completes inside the request.
	if recorder := world.post("/api/v1/admin/returns/"+id+"/approve", `{"reason":"within policy"}`); recorder.Code != http.StatusOK {
		t.Fatalf("approve status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if world.repository.request.Status != returns.ReturnStatusApproved {
		t.Fatalf("status = %q after approval", world.repository.request.Status)
	}

	// Receipt only records the transition and a settlement command; the refund
	// and the restock happen later, off the outbox, so 202 is the honest code.
	recorder := world.post("/api/v1/admin/returns/"+id+"/receive", `{"reason":"warehouse received"}`)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("receive status = %d, want %d; body = %s", recorder.Code, http.StatusAccepted, recorder.Body.String())
	}
	if world.repository.request.Status != returns.ReturnStatusReceived {
		t.Fatalf("status = %q after receipt", world.repository.request.Status)
	}
	if len(world.settlement.events) != 1 || world.settlement.events[0].Topic() != returns.TopicSettlementRequested {
		t.Fatalf("settlement commands = %d, want exactly one", len(world.settlement.events))
	}
}

func TestAnAdminTransitionIsIdempotentUnderRetry(t *testing.T) {
	world := newWorld(t)
	world.seedReturn(returns.ReturnStatusNew)
	id := world.repository.request.ID.String()

	for range 2 {
		if recorder := world.post("/api/v1/admin/returns/"+id+"/approve", `{}`); recorder.Code != http.StatusOK {
			t.Fatalf("approve status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
	}
	if len(world.status.events) != 1 {
		t.Fatalf("status events = %d, want one; a retried approval duplicated history", len(world.status.events))
	}
}

func TestAnAdminTransitionRejectsAMalformedIdentifierAndBody(t *testing.T) {
	world := newWorld(t)
	world.seedReturn(returns.ReturnStatusNew)

	for name, target := range map[string]struct{ path, body string }{
		"not a uuid":  {"/api/v1/admin/returns/not-a-uuid/approve", `{}`},
		"nil uuid":    {"/api/v1/admin/returns/00000000-0000-0000-0000-000000000000/approve", `{}`},
		"bad body":    {"/api/v1/admin/returns/" + world.repository.request.ID.String() + "/approve", `{`},
		"long reason": {"/api/v1/admin/returns/" + world.repository.request.ID.String() + "/approve", `{"reason":"` + strings.Repeat("a", 2001) + `"}`},
	} {
		t.Run(name, func(t *testing.T) {
			if recorder := world.post(target.path, target.body); recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
			if world.repository.request.Status != returns.ReturnStatusNew {
				t.Fatalf("status changed to %q on a malformed request", world.repository.request.Status)
			}
		})
	}
}

func TestAnAdminTransitionWithAnEmptyBodyIsAccepted(t *testing.T) {
	// The reason is optional, so an empty body is a valid approval and must not
	// be read as malformed JSON.
	world := newWorld(t)
	world.seedReturn(returns.ReturnStatusNew)

	recorder := world.postRaw("/api/v1/admin/returns/"+world.repository.request.ID.String()+"/approve", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
}

func TestAMissingReturnIsReportedAsNotFound(t *testing.T) {
	world := newWorld(t)

	recorder := world.post("/api/v1/admin/returns/"+uuid.NewString()+"/approve", `{}`)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}

func TestAnAlreadyReceivedReturnCannotBeApproved(t *testing.T) {
	world := newWorld(t)
	world.seedReturn(returns.ReturnStatusReceived)

	recorder := world.post("/api/v1/admin/returns/"+world.repository.request.ID.String()+"/approve", `{}`)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusUnprocessableEntity, recorder.Body.String())
	}
	if world.repository.request.Status != returns.ReturnStatusReceived {
		t.Fatalf("status = %q, want the receipt to stand", world.repository.request.Status)
	}
}

// world wires the real service and the real eligibility policy behind the
// routes, with an order snapshot the default request is eligible against.
type world struct {
	router                         *gin.Engine
	repository                     *repositoryStub
	orders                         *ordersStub
	authorizer                     *authorizerStub
	status, settlement             *publisherStub
	customerID, orderID, variantID uuid.UUID
	now                            time.Time
	anonymous                      bool
}

func newWorld(t *testing.T) *world {
	t.Helper()
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	delivered := now.AddDate(0, 0, -2)
	total, err := money.NewMoney(1299, "EUR")
	if err != nil {
		t.Fatal(err)
	}
	w := &world{
		repository: &repositoryStub{},
		authorizer: &authorizerStub{},
		status:     &publisherStub{}, settlement: &publisherStub{},
		customerID: uuid.New(), orderID: uuid.New(), variantID: uuid.New(), now: now,
	}
	w.orders = &ordersStub{snapshot: returns.OrderSnapshot{
		OrderID: w.orderID, CustomerID: &w.customerID, Status: "delivered", DeliveredAt: &delivered, Total: total,
		Items: []returns.OrderItemSnapshot{{VariantID: &w.variantID, Quantity: 2, Total: total}},
	}}

	policy, err := returns.NewWindowEligibilityPolicy(14)
	if err != nil {
		t.Fatal(err)
	}
	service, err := returnsApp.NewReturnService(w.repository, w.orders, policy, transactionStub{}, w.status, w.settlement)
	if err != nil {
		t.Fatal(err)
	}

	renderer := apiresponse.NewErrorRenderer(nil)
	w.router = gin.New()
	w.router.Use(renderer.Middleware())
	authenticate := func(c *gin.Context) {
		if !w.anonymous {
			c.Set("user_id", w.customerID)
		}
	}
	v1 := w.router.Group("/api/v1")
	RegisterV1CustomerRoutes(v1, service, authenticate, renderer)
	admin := v1.Group("/admin")
	admin.Use(authenticate)
	RegisterV1AdminRoutes(admin, w.authorizer, service, renderer)
	return w
}

func (w *world) post(path, body string) *httptest.ResponseRecorder {
	return w.postRaw(path, body)
}

func (w *world) postRaw(path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	w.router.ServeHTTP(recorder, request)
	return recorder
}

// seedReturn stores one return request in the given status, as an earlier
// request would have left it.
func (w *world) seedReturn(status returns.ReturnStatus) {
	actor := w.customerID
	request, err := returns.NewReturnRequest(w.orderID, w.customerID, returns.RefundModeFull,
		[]returns.ReturnItem{{VariantID: w.variantID, Quantity: 1, Condition: returns.ItemConditionUnopened}},
		returns.Actor{Type: returns.ActorTypeCustomer, ID: &actor}, w.now)
	if err != nil {
		panic(err)
	}
	request.Status = status
	w.repository.request = request
}

type repositoryStub struct{ request *returns.ReturnRequest }

func (r *repositoryStub) Create(_ context.Context, request *returns.ReturnRequest) error {
	r.request = request
	return nil
}

func (r *repositoryStub) Get(_ context.Context, id uuid.UUID) (*returns.ReturnRequest, error) {
	if r.request == nil || r.request.ID != id {
		return nil, returns.ErrReturnRequestNotFound
	}
	return r.request, nil
}

func (r *repositoryStub) GetForUpdate(ctx context.Context, id uuid.UUID) (*returns.ReturnRequest, error) {
	return r.Get(ctx, id)
}

func (r *repositoryStub) FindReceivedByOrderForUpdate(_ context.Context, orderID uuid.UUID) (*returns.ReturnRequest, error) {
	if r.request == nil || r.request.OrderID != orderID || r.request.Status != returns.ReturnStatusReceived {
		return nil, returns.ErrReturnRequestNotFound
	}
	return r.request, nil
}

func (r *repositoryStub) Update(_ context.Context, request *returns.ReturnRequest, _ returns.ReturnStatusHistory) error {
	r.request = request
	return nil
}

type ordersStub struct {
	snapshot returns.OrderSnapshot
	err      error
	calls    int
}

func (o *ordersStub) GetOrderSnapshot(context.Context, uuid.UUID) (returns.OrderSnapshot, error) {
	o.calls++
	return o.snapshot, o.err
}

type transactionStub struct{}

func (transactionStub) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type publisherStub struct{ events []coreEvents.DomainEvent }

func (p *publisherStub) Publish(_ context.Context, event coreEvents.DomainEvent) error {
	p.events = append(p.events, event)
	return nil
}

type authorizerStub struct {
	permission string
	err        error
}

func (a *authorizerStub) Require(_ context.Context, _ uuid.UUID, permission string) error {
	a.permission = permission
	return a.err
}

func ptr[T any](value T) *T { return &value }

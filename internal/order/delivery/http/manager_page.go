//go:build legacy && ignore
// +build legacy,ignore

package http

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"strconv"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/order/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
	"github.com/gin-gonic/gin"
)

// ManagerPageHandler серверний рендер HTML-сторінки для менеджера
type ManagerPageHandler struct {
	service domain.ManagerService
	l       logger.Logger
	tmpl    *template.Template
}

const managerPageTemplate = `<!DOCTYPE html>
<html lang="uk">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Замовлення №{{.Order.OrderNumber}} — AquaWheel Manager</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{background:#0f172a;font-family:'Segoe UI',system-ui,-apple-system,sans-serif;color:#e2e8f0;min-height:100vh;display:flex;justify-content:center;padding:24px}
.container{max-width:720px;width:100%}
.header{text-align:center;margin-bottom:32px}
.header h1{font-size:28px;font-weight:700;background:linear-gradient(135deg,#0ea5e9,#8b5cf6);-webkit-background-clip:text;-webkit-text-fill-color:transparent;margin-bottom:8px}
.header p{color:#94a3b8;font-size:14px}
.card{background:#1e293b;border-radius:16px;padding:24px;margin-bottom:16px;border:1px solid #334155}
.card-title{font-size:16px;font-weight:600;margin-bottom:16px;color:#f8fafc}
.grid{display:grid;grid-template-columns:1fr 1fr;gap:16px}
.info-card{background:#0f172a;border-radius:12px;padding:16px}
.label{font-size:11px;text-transform:uppercase;color:#94a3b8;font-weight:600;letter-spacing:0.5px;margin-bottom:4px}
.value{font-size:15px;font-weight:500;color:#f1f5f9}
.sublabel{font-size:11px;color:#64748b;margin-top:8px;margin-bottom:4px}
.status-badge{display:inline-block;padding:6px 16px;border-radius:20px;font-size:13px;font-weight:600;color:#fff;background:{{.StatusColor}}}
table{width:100%;border-collapse:collapse}
th{padding:10px 8px;text-align:left;font-size:11px;text-transform:uppercase;color:#94a3b8;font-weight:600;border-bottom:2px solid #334155}
td{color:#e2e8f0;font-size:14px}
.total-row td{padding:14px 8px;font-size:18px;font-weight:700;border-top:2px solid #334155}
.total-amount{color:#10b981}
.ttn-block{background:linear-gradient(135deg,#1e1b4b,#312e81);border-radius:12px;padding:20px;margin-top:16px;text-align:center}
.ttn-number{font-size:24px;font-weight:700;color:#a78bfa;letter-spacing:2px;margin-top:8px}
.actions{display:flex;gap:12px;margin-top:24px}
.btn{flex:1;padding:14px 24px;border:none;border-radius:12px;font-size:15px;font-weight:600;cursor:pointer;transition:all 0.2s}
.btn:disabled{opacity:0.5;cursor:not-allowed}
.btn-primary{background:linear-gradient(135deg,#0ea5e9,#06b6d4);color:#fff;box-shadow:0 4px 14px rgba(14,165,233,0.3)}
.btn-primary:hover:not(:disabled){transform:translateY(-1px);box-shadow:0 6px 20px rgba(14,165,233,0.4)}
.btn-danger{background:linear-gradient(135deg,#ef4444,#dc2626);color:#fff;box-shadow:0 4px 14px rgba(239,68,68,0.3)}
.btn-danger:hover:not(:disabled){transform:translateY(-1px);box-shadow:0 6px 20px rgba(239,68,68,0.4)}
.toast{position:fixed;top:24px;right:24px;padding:16px 24px;border-radius:12px;color:#fff;font-weight:500;font-size:14px;transform:translateX(120%);transition:transform 0.3s;z-index:100}
.toast.show{transform:translateX(0)}
.toast.success{background:#059669}
.toast.error{background:#dc2626}
.spinner{display:inline-block;width:16px;height:16px;border:2px solid rgba(255,255,255,0.3);border-top-color:#fff;border-radius:50%;animation:spin 0.6s linear infinite;margin-right:8px;vertical-align:middle}
@keyframes spin{to{transform:rotate(360deg)}}
@media(max-width:600px){.grid{grid-template-columns:1fr}.actions{flex-direction:column}}
</style>
</head>
<body>
<div class="container">
<div class="header">
<h1>AquaWheel Manager</h1>
<p>Панель обробки замовлення</p>
</div>

<div class="card">
<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:20px">
<div><span style="color:#94a3b8;font-size:13px">Замовлення</span><br><span style="font-size:24px;font-weight:700">№{{.Order.OrderNumber}}</span></div>
<span class="status-badge">{{.StatusName}}</span>
</div>

<div class="grid">
<div class="info-card">
<div class="label">👤 Клієнт</div>
<div class="value">{{.Order.FirstName}} {{.Order.LastName}}</div>
<div class="sublabel">Email</div>
<div class="value" style="font-size:13px">{{.Order.Email}}</div>
</div>
<div class="info-card">
<div class="label">📞 Телефон</div>
<div class="value">{{.Order.Phone}}</div>
<div class="sublabel">Дата замовлення</div>
<div class="value" style="font-size:13px">{{.CreatedAt}}</div>
</div>
{{if .Order.Delivery}}
<div class="info-card">
<div class="label">📦 Доставка</div>
<div class="value">{{.Order.Delivery.Provider}}</div>
<div class="sublabel">Місто: {{.Order.Delivery.CityName}}</div>
<div class="value">{{.Order.Delivery.WarehouseName}}</div>
</div>
{{end}}
</div>
{{if .Order.TTNNumber}}
<div class="ttn-block">
<div class="label">📋 ТТН створено</div>
<div class="ttn-number">{{.Order.TTNNumber}}</div>
</div>
{{end}}
</div>

<div class="card">
<div class="card-title">🛒 Товари</div>
<table>
<thead><tr>
<th>Товар</th><th style="text-align:center">К-сть</th><th style="text-align:right">Ціна</th><th style="text-align:right">Сума</th>
</tr></thead>
<tbody>
{{range .Items}}
<tr>
<td style="padding:12px 8px;border-bottom:1px solid #f1f5f9;"><div style="display:flex;align-items:center;">
{{if .ImageURL}}<img src="{{.ImageURL}}" alt="" style="width:48px;height:48px;object-fit:cover;border-radius:8px;margin-right:12px;">{{end}}
<span>{{.ProductName}}</span></div></td>
<td style="padding:12px 8px;border-bottom:1px solid #f1f5f9;text-align:center;color:#64748b;">{{.Quantity}}</td>
<td style="padding:12px 8px;border-bottom:1px solid #f1f5f9;text-align:right;color:#64748b;">{{.Price}} ₴</td>
<td style="padding:12px 8px;border-bottom:1px solid #f1f5f9;text-align:right;font-weight:600;">{{.TotalPrice}} ₴</td>
</tr>
{{end}}
</tbody>
<tfoot><tr class="total-row">
<td colspan="3" style="text-align:right">Разом:</td>
<td class="total-amount" style="text-align:right">{{.TotalPrice}} ₴</td>
</tr></tfoot>
</table>
</div>

{{if and (eq .Order.StatusID 3) (eq .Order.TTNNumber "")}}
<div class="actions">
<button id="btn-confirm" class="btn btn-primary" onclick="confirmOrder({{.Order.OrderNumber}}, '{{.Token}}')">📋 Підтвердити замовлення</button>
<button id="btn-cancel" class="btn btn-danger" onclick="cancelOrder({{.Order.OrderNumber}}, '{{.Token}}')">❌ Відхилити замовлення</button>
</div>
{{end}}

<div id="toast" class="toast"></div>
</div>

<script>
function showToast(msg,type){var t=document.getElementById('toast');t.textContent=msg;t.className='toast '+type+' show';setTimeout(function(){t.className='toast'},4000)}

function confirmOrder(num,tok){
var btn=document.getElementById('btn-confirm');
btn.disabled=true;btn.innerHTML='<span class="spinner"></span>Підтверджуємо...';
fetch('/api/manager/orders/'+num+'/confirm?token='+encodeURIComponent(tok),{method:'POST'})
.then(function(r){return r.json().then(function(d){return{ok:r.ok,data:d}})})
.then(function(res){
if(res.ok){showToast('Замовлення підтверджено, ТТН '+res.data.ttn_number+' створено!','success');setTimeout(function(){location.reload()},2000)}
else{showToast(res.data.message||'Помилка','error');btn.disabled=false;btn.innerHTML='📋 Підтвердити замовлення'}
}).catch(function(){showToast('Помилка з\'єднання','error');btn.disabled=false;btn.innerHTML='📋 Підтвердити замовлення'})
}

function cancelOrder(num,tok){
if(!confirm('Ви впевнені, що хочете скасувати замовлення?'))return;
var btn=document.getElementById('btn-cancel');
btn.disabled=true;btn.innerHTML='<span class="spinner"></span>Скасовуємо...';
fetch('/api/manager/orders/'+num+'/cancel?token='+encodeURIComponent(tok),{method:'POST'})
.then(function(r){return r.json().then(function(d){return{ok:r.ok,data:d}})})
.then(function(res){
if(res.ok){showToast('Замовлення скасовано','success');setTimeout(function(){location.reload()},2000)}
else{showToast(res.data.message||'Помилка','error');btn.disabled=false;btn.innerHTML='❌ Відхилити замовлення'}
}).catch(function(){showToast('Помилка з\'єднання','error');btn.disabled=false;btn.innerHTML='❌ Відхилити замовлення'})
}
</script>
</body>
</html>`

// NewManagerPageHandler створює новий інстанс
func NewManagerPageHandler(s domain.ManagerService, l logger.Logger) *ManagerPageHandler {
	tmpl := template.Must(template.New("manager").Parse(managerPageTemplate))
	return &ManagerPageHandler{service: s, l: l, tmpl: tmpl}
}

// RenderManagerPage рендерить lightweight HTML-сторінку для менеджера
func (h *ManagerPageHandler) RenderManagerPage(c *gin.Context) {
	orderNumberStr := c.Param("orderNumber")
	orderNumber, err := strconv.ParseInt(orderNumberStr, 10, 64)
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid order number")
		return
	}

	token := c.Query("token")
	if token == "" {
		c.String(http.StatusUnauthorized, "Token is required")
		return
	}

	order, err := h.service.GetOrderByToken(c.Request.Context(), orderNumber, token)
	if err != nil {
		c.String(http.StatusUnauthorized, "Invalid or expired token")
		return
	}

	html, err := h.buildHTML(order, token)
	if err != nil {
		h.l.Errorw("failed to render manager template", "error", err)
		c.String(http.StatusInternalServerError, "Internal Server Error")
		return
	}

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

type templateItem struct {
	ProductName string
	ImageURL    string
	Quantity    int
	Price       string
	TotalPrice  string
}

// buildHTML формує HTML-сторінку
func (h *ManagerPageHandler) buildHTML(order *domain.Order, token string) (string, error) {
	// Status
	statusName := order.Status.Code
	if order.Status.Name != nil {
		if name, ok := order.Status.Name["uk"]; ok {
			statusName = name
		}
	}

	statusColor := "#0ea5e9"
	switch order.StatusID {
	case domain.StatusPaid:
		statusColor = "#10b981"
	case domain.StatusProcessing:
		statusColor = "#f59e0b"
	case domain.StatusShipped:
		statusColor = "#8b5cf6"
	case domain.StatusCancelled:
		statusColor = "#ef4444"
	}

	// Items
	var items []templateItem
	for _, item := range order.Items {
		productName := fmt.Sprintf("Variation %s", item.VariationID.String()[:8])
		imageURL := ""

		for _, trans := range item.Variation.Product.Translations {
			if trans.LanguageCode == "uk" {
				productName = trans.Name
				break
			}
		}
		for _, img := range item.Variation.Product.Images {
			if img.IsPrimary {
				imageURL = img.ImageURL
				break
			}
			if imageURL == "" {
				imageURL = img.ImageURL
			}
		}

		items = append(items, templateItem{
			ProductName: productName,
			ImageURL:    imageURL,
			Quantity:    item.Quantity,
			Price:       fmt.Sprintf("%.2f", float64(item.Price)/100),
			TotalPrice:  fmt.Sprintf("%.2f", float64(item.TotalPrice)/100),
		})
	}

	data := struct {
		Order       *domain.Order
		Token       string
		StatusName  string
		StatusColor string
		CreatedAt   string
		TotalPrice  string
		Items       []templateItem
	}{
		Order:       order,
		Token:       token,
		StatusName:  statusName,
		StatusColor: statusColor,
		CreatedAt:   order.CreatedAt.Format("02.01.2006 15:04"),
		TotalPrice:  fmt.Sprintf("%.2f", float64(order.TotalPrice)/100),
		Items:       items,
	}

	var buf bytes.Buffer
	if err := h.tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

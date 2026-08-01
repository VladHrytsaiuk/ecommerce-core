package email

import "fmt"

// ==========================================
// HTML Email Templates
// ==========================================

// buildVerificationEmailHTML формує гарний HTML-лист для верифікації email.
func buildVerificationEmailHTML(logoURL, code, link string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="uk">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Підтвердження Email</title>
</head>
<body style="margin:0;padding:0;background-color:#f0f4f8;font-family:'Segoe UI',Roboto,'Helvetica Neue',Arial,sans-serif;">
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background-color:#f0f4f8;padding:40px 0;">
<tr>
<td align="center">
<table role="presentation" width="600" cellpadding="0" cellspacing="0" style="background-color:#ffffff;border-radius:16px;overflow:hidden;box-shadow:0 4px 24px rgba(0,0,0,0.08);">

<!-- Header -->
<tr>
<td style="background:linear-gradient(135deg,#0ea5e9 0%%,#06b6d4 50%%,#0891b2 100%%);padding:40px 40px 35px;text-align:center;">
  <table role="presentation" cellpadding="0" cellspacing="0" style="margin:0 auto;">
  <tr>
    <td style="background:#ffffff;background:linear-gradient(180deg,#ffffff 0%%,#f8fafc 100%%);padding:12px 28px;border-radius:14px;text-align:center;box-shadow:0 4px 12px rgba(0,0,0,0.12);">
      <img src="%s" alt="AquaWheel" width="180" style="display:block;margin:0 auto;max-width:100%%;height:auto;border:none;outline:none;text-decoration:none;">
    </td>
  </tr>
  </table>
</td>
</tr>

<!-- Body -->
<tr>
<td style="padding:40px 40px 32px;">
  <h1 style="margin:0 0 16px;color:#0f172a;font-size:24px;font-weight:700;">Підтвердіть вашу пошту</h1>
  <p style="margin:0 0 32px;color:#64748b;font-size:15px;line-height:1.6;">
    Дякуємо за реєстрацію в <strong>AquaWheel Store</strong>! Щоб завершити налаштування акаунту та активувати профіль, натисніть кнопку нижче.
  </p>

  <!-- CTA Button -->
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0">
  <tr>
  <td align="center">
    <a href="%s" target="_blank" style="display:inline-block;background:linear-gradient(135deg,#0ea5e9,#06b6d4);color:#ffffff;text-decoration:none;font-size:16px;font-weight:600;padding:16px 45px;border-radius:12px;box-shadow:0 4px 14px rgba(14,165,233,0.35);">
      ✉️ Підтвердити Email
    </a>
  </td>
  </tr>
  </table>
</td>
</tr>

<!-- Info -->
<tr>
<td style="padding:0 40px 32px;">
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0">
  <tr>
  <td style="background-color:#f8fafc;border-left:4px solid #0ea5e9;border-radius:0 8px 8px 0;padding:14px 16px;">
    <p style="margin:0;color:#475569;font-size:13px;line-height:1.5;">
      ⏱ Посилання дійсне <strong>15 хвилин</strong>. Якщо ви не реєструвалися у AquaWheel Store — просто ігноруйте цей лист.
    </p>
  </td>
  </tr>
  </table>
</td>
</tr>

<!-- Divider -->
<tr>
<td style="padding:0 40px;">
  <hr style="border:none;border-top:1px solid #e2e8f0;margin:0;"/>
</td>
</tr>

<!-- Footer -->
<tr>
<td style="padding:24px 40px 32px;text-align:center;">
  <p style="margin:0 0 4px;color:#94a3b8;font-size:12px;">© 2026 AquaWheel Store. Усі права захищені.</p>
  <p style="margin:0;color:#cbd5e1;font-size:11px;">Цей лист надіслано автоматично. Будь ласка, не відповідайте на нього.</p>
</td>
</tr>

</table>
</td>
</tr>
</table>
</body>
</html>`, logoURL, link)
}

// buildPasswordResetEmailHTML формує гарний HTML-лист для скидання пароля.
func buildPasswordResetEmailHTML(logoURL, code, link string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="uk">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Скидання пароля</title>
</head>
<body style="margin:0;padding:0;background-color:#f0f4f8;font-family:'Segoe UI',Roboto,'Helvetica Neue',Arial,sans-serif;">
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background-color:#f0f4f8;padding:40px 0;">
<tr>
<td align="center">
<table role="presentation" width="600" cellpadding="0" cellspacing="0" style="background-color:#ffffff;border-radius:16px;overflow:hidden;box-shadow:0 4px 24px rgba(0,0,0,0.08);">

<!-- Header -->
<tr>
<td style="background:linear-gradient(135deg,#f97316 0%%,#ef4444 50%%,#dc2626 100%%);padding:40px 40px 35px;text-align:center;">
  <table role="presentation" cellpadding="0" cellspacing="0" style="margin:0 auto;">
  <tr>
    <td style="background:#ffffff;background:linear-gradient(180deg,#ffffff 0%%,#f8fafc 100%%);padding:12px 28px;border-radius:14px;text-align:center;box-shadow:0 4px 12px rgba(0,0,0,0.12);">
      <img src="%s" alt="AquaWheel" width="180" style="display:block;margin:0 auto;max-width:100%%;height:auto;border:none;outline:none;text-decoration:none;">
    </td>
  </tr>
  </table>
</td>
</tr>

<!-- Body -->
<tr>
<td style="padding:40px 40px 32px;">
  <h1 style="margin:0 0 16px;color:#0f172a;font-size:24px;font-weight:700;">Скидання пароля</h1>
  <p style="margin:0 0 32px;color:#64748b;font-size:15px;line-height:1.6;">
    Ми отримали запит на скидання пароля для вашого акаунту <strong>AquaWheel Store</strong>. Щоб встановити новий пароль, натисніть кнопку нижче.
  </p>

  <!-- CTA Button -->
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0">
  <tr>
  <td align="center">
    <a href="%s" target="_blank" style="display:inline-block;background:linear-gradient(135deg,#f97316,#ef4444);color:#ffffff;text-decoration:none;font-size:16px;font-weight:600;padding:16px 45px;border-radius:12px;box-shadow:0 4px 14px rgba(239,68,68,0.35);">
      🔑 Скинути пароль
    </a>
  </td>
  </tr>
  </table>
</td>
</tr>

<!-- Security Warning -->
<tr>
<td style="padding:0 40px 32px;">
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0">
  <tr>
  <td style="background-color:#fef2f2;border-left:4px solid #ef4444;border-radius:0 8px 8px 0;padding:14px 16px;">
    <p style="margin:0;color:#991b1b;font-size:13px;line-height:1.5;">
      🛡️ Посилання дійсне <strong>15 хвилин</strong>. Якщо ви не запитували скидання пароля — просто ігноруйте цей лист.
    </p>
  </td>
  </tr>
  </table>
</td>
</tr>

<!-- Divider -->
<tr>
<td style="padding:0 40px;">
  <hr style="border:none;border-top:1px solid #e2e8f0;margin:0;"/>
</td>
</tr>

<!-- Footer -->
<tr>
<td style="padding:24px 40px 32px;text-align:center;">
  <p style="margin:0 0 4px;color:#94a3b8;font-size:12px;">© 2026 AquaWheel Store. Усі права захищені.</p>
  <p style="margin:0;color:#cbd5e1;font-size:11px;">Цей лист надіслано автоматично. Будь ласка, не відповідайте на нього.</p>
</td>
</tr>

</table>
</td>
</tr>
</table>
</body>
</html>`, logoURL, link)
}

// buildOrderConfirmationEmailHTML формує HTML-лист для підтвердження замовлення клієнту.
func buildOrderConfirmationEmailHTML(logoURL string, data OrderEmailData) string {
	// Формуємо рядки товарів
	itemsHTML := ""
	for _, item := range data.Items {
		itemsHTML += fmt.Sprintf(`<tr>
<td style="padding:10px 0;border-bottom:1px solid #f1f5f9;color:#334155;font-size:14px;">%s</td>
<td style="padding:10px 0;border-bottom:1px solid #f1f5f9;color:#64748b;font-size:14px;text-align:center;">%d</td>
<td style="padding:10px 0;border-bottom:1px solid #f1f5f9;color:#64748b;font-size:14px;text-align:right;">%.2f ₴</td>
<td style="padding:10px 0;border-bottom:1px solid #f1f5f9;color:#0f172a;font-size:14px;font-weight:600;text-align:right;">%.2f ₴</td>
</tr>`, item.Name, item.Quantity, float64(item.Price)/100, float64(item.Total)/100)
	}

	newUserBlock := ""
	if data.IsNewUser {
		newUserBlock = `<tr>
<td style="padding:0 40px 24px;">
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0">
  <tr>
  <td style="background-color:#eff6ff;border-left:4px solid #3b82f6;border-radius:0 8px 8px 0;padding:14px 16px;">
    <p style="margin:0;color:#1e40af;font-size:13px;line-height:1.5;">
      🔑 Для вас створено акаунт. Для входу скористайтесь функцією <strong>«Забув пароль»</strong> на сторінці авторизації.
    </p>
  </td>
  </tr>
  </table>
</td>
</tr>`
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="uk">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Підтвердження замовлення</title>
</head>
<body style="margin:0;padding:0;background-color:#f0f4f8;font-family:'Segoe UI',Roboto,'Helvetica Neue',Arial,sans-serif;">
<table role="presentation" width="100%%%%" cellpadding="0" cellspacing="0" style="background-color:#f0f4f8;padding:40px 0;">
<tr>
<td align="center">
<table role="presentation" width="600" cellpadding="0" cellspacing="0" style="background-color:#ffffff;border-radius:16px;overflow:hidden;box-shadow:0 4px 24px rgba(0,0,0,0.08);">

<!-- Header -->
<tr>
<td style="background:linear-gradient(135deg,#10b981 0%%,#059669 50%%,#047857 100%%);padding:40px 40px 35px;text-align:center;">
  <table role="presentation" cellpadding="0" cellspacing="0" style="margin:0 auto;">
  <tr>
    <td style="background:#ffffff;padding:12px 28px;border-radius:14px;text-align:center;box-shadow:0 4px 12px rgba(0,0,0,0.12);">
      <img src="%s" alt="AquaWheel" width="180" style="display:block;margin:0 auto;max-width:100%%%%;height:auto;">
    </td>
  </tr>
  </table>
</td>
</tr>

<!-- Body -->
<tr>
<td style="padding:40px 40px 24px;">
  <h1 style="margin:0 0 8px;color:#0f172a;font-size:24px;font-weight:700;">✅ Замовлення №%d оформлено!</h1>
  <p style="margin:0 0 24px;color:#64748b;font-size:15px;line-height:1.6;">
    %s, дякуємо за ваше замовлення в <strong>AquaWheel Store</strong>. Нижче — деталі вашого замовлення.
  </p>

  <!-- Items Table -->
  <table role="presentation" width="100%%%%" cellpadding="0" cellspacing="0" style="margin-bottom:24px;">
  <tr>
    <td style="padding:8px 0;border-bottom:2px solid #e2e8f0;color:#94a3b8;font-size:12px;text-transform:uppercase;font-weight:600;">Товар</td>
    <td style="padding:8px 0;border-bottom:2px solid #e2e8f0;color:#94a3b8;font-size:12px;text-transform:uppercase;font-weight:600;text-align:center;">К-сть</td>
    <td style="padding:8px 0;border-bottom:2px solid #e2e8f0;color:#94a3b8;font-size:12px;text-transform:uppercase;font-weight:600;text-align:right;">Ціна</td>
    <td style="padding:8px 0;border-bottom:2px solid #e2e8f0;color:#94a3b8;font-size:12px;text-transform:uppercase;font-weight:600;text-align:right;">Сума</td>
  </tr>
  %s
  <tr>
    <td colspan="3" style="padding:12px 0;color:#0f172a;font-size:16px;font-weight:700;text-align:right;">Разом:</td>
    <td style="padding:12px 0;color:#059669;font-size:16px;font-weight:700;text-align:right;">%.2f ₴</td>
  </tr>
  </table>
</td>
</tr>

<!-- Delivery -->
<tr>
<td style="padding:0 40px 24px;">
  <table role="presentation" width="100%%%%" cellpadding="0" cellspacing="0">
  <tr>
  <td style="background-color:#f8fafc;border-radius:8px;padding:14px 16px;">
    <p style="margin:0 0 4px;color:#94a3b8;font-size:12px;text-transform:uppercase;font-weight:600;">📦 Доставка</p>
    <p style="margin:0;color:#334155;font-size:14px;line-height:1.5;">%s — %s, %s</p>
  </td>
  </tr>
  </table>
</td>
</tr>

<!-- New User Block (conditional) -->
%s

<!-- Divider -->
<tr>
<td style="padding:0 40px;">
  <hr style="border:none;border-top:1px solid #e2e8f0;margin:0;"/>
</td>
</tr>

<!-- Footer -->
<tr>
<td style="padding:24px 40px 32px;text-align:center;">
  <p style="margin:0 0 16px;color:#94a3b8;font-size:12px;line-height:1.5;">Якщо ви не робили цього замовлення або помітили помилку в даних, будь ласка, зв'яжіться з нашим менеджером.</p>
  <p style="margin:0 0 4px;color:#94a3b8;font-size:12px;">© 2026 AquaWheel Store. Усі права захищені.</p>
  <p style="margin:0;color:#cbd5e1;font-size:11px;">Цей лист надіслано автоматично. Будь ласка, не відповідайте на нього.</p>
</td>
</tr>

</table>
</td>
</tr>
</table>
</body>
</html>`,
		logoURL,
		data.OrderNumber,
		data.CustomerName,
		itemsHTML,
		float64(data.TotalPrice)/100,
		data.Delivery.Provider, data.Delivery.CityName, data.Delivery.WarehouseName,
		newUserBlock,
	)
}

// buildAdminOrderNotificationEmailHTML формує HTML-лист для сповіщення адміністратора.
func buildAdminOrderNotificationEmailHTML(logoURL string, data OrderEmailData) string {
	itemsHTML := ""
	for _, item := range data.Items {
		itemsHTML += fmt.Sprintf(`<tr>
<td style="padding:8px 4px;border-bottom:1px solid #f1f5f9;font-size:13px;color:#334155;">%s</td>
<td style="padding:8px 4px;border-bottom:1px solid #f1f5f9;font-size:13px;color:#64748b;">%s</td>
<td style="padding:8px 4px;border-bottom:1px solid #f1f5f9;font-size:13px;color:#64748b;text-align:center;">%d</td>
<td style="padding:8px 4px;border-bottom:1px solid #f1f5f9;font-size:13px;color:#0f172a;font-weight:600;text-align:right;">%.2f ₴</td>
</tr>`, item.Name, item.SKU, item.Quantity, float64(item.Total)/100)
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="uk">
<head><meta charset="UTF-8"><title>Нове замовлення</title></head>
<body style="margin:0;padding:0;background-color:#f0f4f8;font-family:'Segoe UI',Roboto,Arial,sans-serif;">
<table width="100%%%%" cellpadding="0" cellspacing="0" style="background-color:#f0f4f8;padding:30px 0;">
<tr><td align="center">
<table width="600" cellpadding="0" cellspacing="0" style="background-color:#ffffff;border-radius:12px;overflow:hidden;box-shadow:0 2px 12px rgba(0,0,0,0.06);">

<tr><td style="background:#1e293b;padding:24px 32px;">
  <h1 style="margin:0;color:#ffffff;font-size:20px;">🛒 Нове замовлення №%d</h1>
</td></tr>

<tr><td style="padding:24px 32px;">
  <table width="100%%%%" cellpadding="0" cellspacing="0" style="margin-bottom:20px;">
    <tr><td style="color:#94a3b8;font-size:12px;padding:4px 0;">Клієнт:</td><td style="color:#0f172a;font-size:14px;padding:4px 0;font-weight:600;">%s</td></tr>
    <tr><td style="color:#94a3b8;font-size:12px;padding:4px 0;">Email:</td><td style="color:#0f172a;font-size:14px;padding:4px 0;">%s</td></tr>
    <tr><td style="color:#94a3b8;font-size:12px;padding:4px 0;">Телефон:</td><td style="color:#0f172a;font-size:14px;padding:4px 0;">%s</td></tr>
    <tr><td style="color:#94a3b8;font-size:12px;padding:4px 0;">Доставка:</td><td style="color:#0f172a;font-size:14px;padding:4px 0;">%s — %s, %s</td></tr>
  </table>

  <table width="100%%%%" cellpadding="0" cellspacing="0">
    <tr>
      <td style="padding:8px 4px;border-bottom:2px solid #e2e8f0;color:#94a3b8;font-size:11px;text-transform:uppercase;">Товар</td>
      <td style="padding:8px 4px;border-bottom:2px solid #e2e8f0;color:#94a3b8;font-size:11px;text-transform:uppercase;">SKU</td>
      <td style="padding:8px 4px;border-bottom:2px solid #e2e8f0;color:#94a3b8;font-size:11px;text-transform:uppercase;text-align:center;">К-сть</td>
      <td style="padding:8px 4px;border-bottom:2px solid #e2e8f0;color:#94a3b8;font-size:11px;text-transform:uppercase;text-align:right;">Сума</td>
    </tr>
    %s
    <tr>
      <td colspan="3" style="padding:10px 4px;font-size:15px;font-weight:700;text-align:right;color:#0f172a;">РАЗОМ:</td>
      <td style="padding:10px 4px;font-size:15px;font-weight:700;text-align:right;color:#059669;">%.2f ₴</td>
    </tr>
  </table>
</td></tr>

%s

<tr><td style="padding:16px 32px 24px;text-align:center;">
  <p style="margin:0;color:#94a3b8;font-size:11px;">AquaWheel Store — Admin Notification</p>
</td></tr>

</table>
</td></tr>
</table>
</body>
</html>`,
		data.OrderNumber,
		data.CustomerName, data.Email, data.Phone,
		data.Delivery.Provider, data.Delivery.CityName, data.Delivery.WarehouseName,
		itemsHTML,
		float64(data.TotalPrice)/100,
		buildManagerLinkBlock(data.ManagerLink),
	)
}

// buildSecurityWarningEmailHTML формує HTML-лист з попередженням про використання email в замовленні.
func buildSecurityWarningEmailHTML(logoURL string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="uk">
<head><meta charset="UTF-8"><title>Попередження безпеки</title></head>
<body style="margin:0;padding:0;background-color:#f0f4f8;font-family:'Segoe UI',Roboto,Arial,sans-serif;">
<table width="100%%%%" cellpadding="0" cellspacing="0" style="background-color:#f0f4f8;padding:40px 0;">
<tr><td align="center">
<table width="600" cellpadding="0" cellspacing="0" style="background-color:#ffffff;border-radius:16px;overflow:hidden;box-shadow:0 4px 24px rgba(0,0,0,0.08);">

<tr>
<td style="background:linear-gradient(135deg,#f59e0b 0%%,#d97706 100%%);padding:40px 40px 35px;text-align:center;">
  <table cellpadding="0" cellspacing="0" style="margin:0 auto;">
  <tr>
    <td style="background:#ffffff;padding:12px 28px;border-radius:14px;text-align:center;box-shadow:0 4px 12px rgba(0,0,0,0.12);">
      <img src="%s" alt="AquaWheel" width="180" style="display:block;margin:0 auto;max-width:100%%%%;height:auto;">
    </td>
  </tr>
  </table>
</td>
</tr>

<tr>
<td style="padding:40px 40px 32px;">
  <h1 style="margin:0 0 16px;color:#0f172a;font-size:24px;font-weight:700;">⚠️ Замовлення з вашим Email</h1>
  <p style="margin:0 0 24px;color:#64748b;font-size:15px;line-height:1.6;">
    Було створено замовлення в <strong>AquaWheel Store</strong> з використанням вашої електронної адреси.
  </p>
  <table width="100%%%%" cellpadding="0" cellspacing="0">
  <tr>
  <td style="background-color:#fef2f2;border-left:4px solid #ef4444;border-radius:0 8px 8px 0;padding:14px 16px;">
    <p style="margin:0;color:#991b1b;font-size:14px;line-height:1.5;">
      Якщо це <strong>не ви</strong> — будь ласка, зверніться до нашої служби підтримки для з'ясування ситуації.
    </p>
  </td>
  </tr>
  </table>
</td>
</tr>

<tr><td style="padding:0 40px;"><hr style="border:none;border-top:1px solid #e2e8f0;margin:0;"/></td></tr>

<tr>
<td style="padding:24px 40px 32px;text-align:center;">
  <p style="margin:0 0 4px;color:#94a3b8;font-size:12px;">© 2026 AquaWheel Store. Усі права захищені.</p>
</td>
</tr>

</table>
</td></tr>
</table>
</body>
</html>`, logoURL)
}

// buildManagerLinkBlock формує HTML-блок з кнопкою для менеджера (якщо посилання є).
func buildManagerLinkBlock(link string) string {
	if link == "" {
		return ""
	}
	return fmt.Sprintf(`<tr><td style="padding:0 32px 16px;">
  <table width="100%%%%" cellpadding="0" cellspacing="0">
  <tr><td align="center" style="padding:16px 0;">
    <a href="%s" target="_blank" style="display:inline-block;background:linear-gradient(135deg,#0ea5e9,#06b6d4);color:#ffffff;text-decoration:none;font-size:14px;font-weight:600;padding:12px 32px;border-radius:10px;box-shadow:0 4px 14px rgba(14,165,233,0.35);">
      📋 Переглянути замовлення
    </a>
  </td></tr>
  </table>
</td></tr>`, link)
}

// buildShipmentCreatedEmailHTML формує HTML-лист для клієнта про створення ТТН.
func buildShipmentCreatedEmailHTML(logoURL string, data ShipmentEmailData) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="uk">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>Замовлення відправлено</title></head>
<body style="margin:0;padding:0;background-color:#f0f4f8;font-family:'Segoe UI',Roboto,'Helvetica Neue',Arial,sans-serif;">
<table role="presentation" width="100%%%%" cellpadding="0" cellspacing="0" style="background-color:#f0f4f8;padding:40px 0;">
<tr><td align="center">
<table role="presentation" width="600" cellpadding="0" cellspacing="0" style="background-color:#ffffff;border-radius:16px;overflow:hidden;box-shadow:0 4px 24px rgba(0,0,0,0.08);">

<!-- Header -->
<tr>
<td style="background:linear-gradient(135deg,#8b5cf6 0%%,#7c3aed 50%%,#6d28d9 100%%);padding:40px 40px 35px;text-align:center;">
  <table role="presentation" cellpadding="0" cellspacing="0" style="margin:0 auto;">
  <tr>
    <td style="background:#ffffff;padding:12px 28px;border-radius:14px;text-align:center;box-shadow:0 4px 12px rgba(0,0,0,0.12);">
      <img src="%s" alt="AquaWheel" width="180" style="display:block;margin:0 auto;max-width:100%%%%;height:auto;">
    </td>
  </tr>
  </table>
</td>
</tr>

<!-- Body -->
<tr>
<td style="padding:40px 40px 24px;">
  <h1 style="margin:0 0 16px;color:#0f172a;font-size:24px;font-weight:700;">📦 Замовлення №%d відправлено!</h1>
  <p style="margin:0 0 24px;color:#64748b;font-size:15px;line-height:1.6;">
    %s, ваше замовлення вже в дорозі! Нижче — дані для відстеження.
  </p>

  <!-- TTN Info -->
  <table role="presentation" width="100%%%%" cellpadding="0" cellspacing="0" style="margin-bottom:24px;">
  <tr>
  <td style="background-color:#f5f3ff;border-left:4px solid #8b5cf6;border-radius:0 12px 12px 0;padding:20px;">
    <table width="100%%%%" cellpadding="0" cellspacing="0">
    <tr><td style="color:#94a3b8;font-size:12px;text-transform:uppercase;font-weight:600;padding-bottom:8px;">Служба доставки</td></tr>
    <tr><td style="color:#0f172a;font-size:16px;font-weight:600;padding-bottom:16px;">%s</td></tr>
    <tr><td style="color:#94a3b8;font-size:12px;text-transform:uppercase;font-weight:600;padding-bottom:8px;">Номер ТТН (накладна)</td></tr>
    <tr><td style="color:#7c3aed;font-size:20px;font-weight:700;letter-spacing:1px;padding-bottom:16px;">%s</td></tr>
    <tr><td style="color:#94a3b8;font-size:12px;text-transform:uppercase;font-weight:600;padding-bottom:8px;">Пункт видачі</td></tr>
    <tr><td style="color:#0f172a;font-size:14px;">%s, %s</td></tr>
    </table>
  </td>
  </tr>
  </table>

  <!-- Tracking Tip -->
  <table role="presentation" width="100%%%%" cellpadding="0" cellspacing="0">
  <tr>
  <td style="background-color:#f8fafc;border-radius:8px;padding:14px 16px;">
    <p style="margin:0;color:#475569;font-size:13px;line-height:1.5;">
      💡 Відстежуйте посилку на сайті <strong>Нової Пошти</strong> за номером ТТН або у мобільному додатку.
    </p>
  </td>
  </tr>
  </table>
</td>
</tr>

<!-- Divider -->
<tr><td style="padding:0 40px;"><hr style="border:none;border-top:1px solid #e2e8f0;margin:0;"/></td></tr>

<!-- Footer -->
<tr>
<td style="padding:24px 40px 32px;text-align:center;">
  <p style="margin:0 0 4px;color:#94a3b8;font-size:12px;">© 2026 AquaWheel Store. Усі права захищені.</p>
  <p style="margin:0;color:#cbd5e1;font-size:11px;">Цей лист надіслано автоматично. Будь ласка, не відповідайте на нього.</p>
</td>
</tr>

</table>
</td></tr>
</table>
</body>
</html>`,
		logoURL,
		data.OrderNumber,
		data.CustomerName,
		data.Provider,
		data.TrackingNumber,
		data.CityName, data.WarehouseName,
	)
}

// buildFeedbackEmailHTML формує HTML-лист для сповіщення про новий відгук.
func buildFeedbackEmailHTML(logoURL string, feedbackType string, userEmail string, content string, mediaURL *string) string {
	mediaBlock := ""
	if mediaURL != nil && *mediaURL != "" {
		mediaBlock = fmt.Sprintf(`
		<tr>
			<td style="color:#94a3b8;font-size:12px;padding:4px 0;">Прикріплений файл:</td>
			<td style="color:#0f172a;font-size:14px;padding:4px 0;word-break:break-all;">
				<a href="%s" target="_blank" style="color:#0ea5e9;text-decoration:underline;">Переглянути медіа</a>
			</td>
		</tr>`, *mediaURL)
	}

	typeName := "Зворотній зв'язок"
	if feedbackType == "product_improvement" {
		typeName = "Покращення товару 💡"
	} else if feedbackType == "bug" {
		typeName = "Баг на сайті 🐛"
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="uk">
<head><meta charset="UTF-8"><title>Нове звернення зворотнього зв'язку</title></head>
<body style="margin:0;padding:0;background-color:#f0f4f8;font-family:'Segoe UI',Roboto,Arial,sans-serif;">
<table width="100%%%%" cellpadding="0" cellspacing="0" style="background-color:#f0f4f8;padding:30px 0;">
<tr><td align="center">
<table width="600" cellpadding="0" cellspacing="0" style="background-color:#ffffff;border-radius:12px;overflow:hidden;box-shadow:0 2px 12px rgba(0,0,0,0.06);">

<tr><td style="background:#1e293b;padding:24px 32px;">
  <h1 style="margin:0;color:#ffffff;font-size:20px;">🔔 Нове звернення</h1>
</td></tr>

<tr><td style="padding:24px 32px;">
  <table width="100%%%%" cellpadding="0" cellspacing="0" style="margin-bottom:20px;border-collapse:collapse;">
    <tr><td style="color:#94a3b8;font-size:12px;padding:4px 0;width:120px;">Тип звернення:</td><td style="color:#0f172a;font-size:14px;padding:4px 0;font-weight:600;">%s</td></tr>
    <tr><td style="color:#94a3b8;font-size:12px;padding:4px 0;">Email користувача:</td><td style="color:#0f172a;font-size:14px;padding:4px 0;">%s</td></tr>
    %s
  </table>

  <div style="background-color:#f8fafc;border-left:4px solid #0ea5e9;border-radius:0 8px 8px 0;padding:16px;margin-top:20px;">
    <p style="margin:0 0 8px;color:#94a3b8;font-size:11px;text-transform:uppercase;font-weight:600;">Повідомлення:</p>
    <p style="margin:0;color:#334155;font-size:14px;line-height:1.6;white-space:pre-wrap;">%s</p>
  </div>
</td></tr>

<tr><td style="padding:16px 32px 24px;text-align:center;">
  <p style="margin:0;color:#94a3b8;font-size:11px;">AquaWheel Store — Admin Feedback System</p>
</td></tr>

</table>
</td></tr>
</table>
</body>
</html>`, typeName, userEmail, mediaBlock, content)
}


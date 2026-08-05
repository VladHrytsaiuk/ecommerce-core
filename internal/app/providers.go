package app

import (
	"fmt"

	dhlExpressAdapter "github.com/VladHrytsaiuk/ecommerce-core/internal/adapters/delivery/dhlexpress"
	novaposhtaAdapter "github.com/VladHrytsaiuk/ecommerce-core/internal/adapters/delivery/novaposhta"
	liqpayAdapter "github.com/VladHrytsaiuk/ecommerce-core/internal/adapters/payment/liqpay"
	redsysAdapter "github.com/VladHrytsaiuk/ecommerce-core/internal/adapters/payment/redsys"
	stripeAdapter "github.com/VladHrytsaiuk/ecommerce-core/internal/adapters/payment/stripe"
	deliveryApp "github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/application"
	deliveryDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/delivery/domain"
	paymentsApp "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/application"
	paymentsDomain "github.com/VladHrytsaiuk/ecommerce-core/internal/payments/domain"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/config"
)

// newPaymentRegistry is part of the Composition Root. It selects only the
// adapters enabled for this store; core checkout receives the selected port,
// never an SDK or provider configuration.
func newPaymentRegistry(cfg *config.Config, storeConfig StoreConfig) (*paymentsApp.Registry, error) {
	gateways := make([]paymentsDomain.Gateway, 0, len(storeConfig.PaymentProviders))
	for _, code := range storeConfig.PaymentProviders {
		switch code {
		case "liqpay":
			gateway, err := liqpayAdapter.New(liqpayAdapter.Config{PublicKey: cfg.LiqPayPublicKey, PrivateKey: cfg.LiqPayPrivateKey, CallbackURL: cfg.LiqPayCallbackURL, PriceScale: storeConfig.PriceScale})
			if err != nil {
				return nil, err
			}
			gateways = append(gateways, gateway)
		case "stripe":
			gateway, err := stripeAdapter.New(stripeAdapter.Config{SecretKey: cfg.StripeSecretKey, WebhookSecret: cfg.StripeWebhookSecret})
			if err != nil {
				return nil, err
			}
			gateways = append(gateways, gateway)
		case "redsys":
			gateway, err := redsysAdapter.New(redsysAdapter.Config{MerchantCode: cfg.RedsysMerchantCode, Terminal: cfg.RedsysTerminal, SecretKey: cfg.RedsysSecretKey, CallbackURL: cfg.RedsysCallbackURL, Currency: storeConfig.Currency, CurrencyCode: cfg.RedsysCurrencyCode})
			if err != nil {
				return nil, err
			}
			gateways = append(gateways, gateway)
		default:
			return nil, fmt.Errorf("payment adapter %q is not implemented", code)
		}
	}
	return paymentsApp.NewRegistry(storeConfig.PaymentProviders, storeConfig.PaymentDefault, gateways...)
}

func newDeliveryRegistry(cfg *config.Config, storeConfig StoreConfig) (*deliveryApp.Registry, error) {
	carriers := make([]deliveryDomain.Carrier, 0, len(storeConfig.ShippingProviders))
	for _, code := range storeConfig.ShippingProviders {
		switch code {
		case "novaposhta":
			carrier, err := novaposhtaAdapter.New(novaposhtaAdapter.Config{APIKey: cfg.NovaPoshtaAPIKey, BaseURL: cfg.NovaPoshtaURL, SenderRef: cfg.NPSenderRef, SenderCityRef: cfg.NPSenderCityRef, SenderAddressRef: cfg.NPSenderAddressRef, ContactSenderRef: cfg.NPContactSenderRef, SenderPhone: cfg.NPSenderPhone})
			if err != nil {
				return nil, err
			}
			carriers = append(carriers, carrier)
		case "dhlexpress":
			carrier, err := dhlExpressAdapter.New(dhlExpressAdapter.Config{BaseURL: cfg.DHLExpressBaseURL, Username: cfg.DHLExpressUsername, Password: cfg.DHLExpressPassword, AccountNumber: cfg.DHLExpressAccountNumber, ProductCode: cfg.DHLExpressProductCode, SenderName: cfg.DHLExpressSenderName, SenderPhone: cfg.DHLExpressSenderPhone, SenderCountry: cfg.DHLExpressSenderCountry, SenderPostal: cfg.DHLExpressSenderPostal, SenderCity: cfg.DHLExpressSenderCity, SenderLine1: cfg.DHLExpressSenderLine1, PackageLength: cfg.DHLExpressPackageLength, PackageWidth: cfg.DHLExpressPackageWidth, PackageHeight: cfg.DHLExpressPackageHeight, PriceScale: storeConfig.PriceScale})
			if err != nil {
				return nil, err
			}
			carriers = append(carriers, carrier)
		default:
			return nil, fmt.Errorf("delivery adapter %q is not implemented", code)
		}
	}
	return deliveryApp.NewRegistry(storeConfig.ShippingProviders, storeConfig.ShippingDefault, carriers...)
}

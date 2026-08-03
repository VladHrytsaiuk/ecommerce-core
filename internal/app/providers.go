package app

import (
	"fmt"

	liqpayAdapter "github.com/VladHrytsaiuk/ecommerce-core/internal/adapters/payment/liqpay"
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
		default:
			return nil, fmt.Errorf("payment adapter %q is not implemented", code)
		}
	}
	return paymentsApp.NewRegistry(storeConfig.PaymentProviders, storeConfig.PaymentDefault, gateways...)
}

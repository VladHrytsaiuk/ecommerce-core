package ukrposhta_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/integration/ukrposhta"
	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

func TestUkrposhtaClient(t *testing.T) {
	logger.Init()
	client := ukrposhta.NewClient(logger.Log)
	ctx := context.Background()

	t.Run("GetAreas", func(t *testing.T) {
		areas, err := client.GetAreas(ctx)
		assert.Error(t, err)
		assert.Equal(t, "ukrposhta integration not implemented yet", err.Error())
		assert.Nil(t, areas)
	})

	t.Run("GetCities", func(t *testing.T) {
		cities, err := client.GetCities(ctx, "area-ref")
		assert.Error(t, err)
		assert.Equal(t, "ukrposhta integration not implemented yet", err.Error())
		assert.Nil(t, cities)
	})

	t.Run("GetWarehouses", func(t *testing.T) {
		warehouses, err := client.GetWarehouses(ctx, "city-ref", "warehouse-type")
		assert.Error(t, err)
		assert.Equal(t, "ukrposhta integration not implemented yet", err.Error())
		assert.Nil(t, warehouses)
	})
}

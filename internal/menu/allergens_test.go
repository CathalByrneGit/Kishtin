package menu

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllergens(t *testing.T) {
	assert.Len(t, Allergens, 14, "the regulation lists 14")
	got, err := CleanAllergens([]string{" Milk", "gluten", "milk", "SESAME"})
	require.NoError(t, err)
	assert.Equal(t, []string{"gluten", "milk", "sesame"}, got, "once each, in the regulation's order")
	got, err = CleanAllergens(nil)
	require.NoError(t, err)
	assert.Equal(t, []string{}, got)
	_, err = CleanAllergens([]string{"garlic"})
	assert.ErrorContains(t, err, `"garlic" is not one of the 14 allergens`)
	assert.Equal(t, "Sulphur dioxide and sulphites", AllergenName("sulphites"))
	assert.Equal(t, "garlic", AllergenName("garlic"))

	d := Dish{Name: "Soup", Course: CourseStarter, StationID: "s", PlateSeconds: 60, Allergens: []string{"celery", "Gluten"}}.Normalize()
	assert.Equal(t, []string{"gluten", "celery"}, d.Allergens)
	assert.NoError(t, d.Validate())
	d.Allergens = []string{"garlic"}
	assert.Error(t, d.Validate())
}

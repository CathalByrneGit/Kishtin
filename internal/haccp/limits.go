package haccp

import "time"

// Default critical limits.
//
// These are the FSAI defaults Kishtin starts from. A restaurant changes its
// own limits through limit versions (see LimitVersion), never by editing
// these constants. Changing a constant here needs a task that asks for it,
// plus updated tests and AGENTS.md in the same change.
//
// Source for every value below: Food Safety Authority of Ireland, "Safe
// Catering" pack (Your Guide to Making Food Safely), the chilled storage,
// frozen storage, cooking, reheating, hot holding, cooling and delivery
// sections.
const (
	// FSAIFridgeMaxC: chilled food is stored at 5°C or below.
	FSAIFridgeMaxC = 5.0

	// FSAIFreezerMaxC: frozen food is stored at −18°C or below.
	FSAIFreezerMaxC = -18.0

	// FSAICookCoreMinC: cooked food reaches a core temperature of 75°C or above.
	FSAICookCoreMinC = 75.0

	// FSAIReheatCoreMinC: reheated food reaches a core temperature of 70°C or above.
	FSAIReheatCoreMinC = 70.0

	// FSAIHotHoldMinC: hot food is held at 63°C or above.
	FSAIHotHoldMinC = 63.0

	// FSAICoolingIntoFridgeWithin: cooked food goes into the fridge or blast
	// chiller within 2 hours of cooking.
	FSAICoolingIntoFridgeWithin = 2 * time.Hour

	// FSAICoolingCutoffAfter and FSAICoolingCutoffMaxC: food that is not in
	// the fridge in time and is still above 10°C at 2.5 hours is served
	// immediately or discarded.
	FSAICoolingCutoffAfter = 2*time.Hour + 30*time.Minute
	FSAICoolingCutoffMaxC  = 10.0

	// FSAIChilledDeliveryMaxC: chilled deliveries arrive at 5°C or below,
	// borderline up to 8°C.
	FSAIChilledDeliveryMaxC           = 5.0
	FSAIChilledDeliveryBorderlineMaxC = 8.0

	// FSAIFrozenDeliveryMaxC: frozen deliveries arrive at −18°C or below,
	// borderline up to −15°C.
	FSAIFrozenDeliveryMaxC           = -18.0
	FSAIFrozenDeliveryBorderlineMaxC = -15.0
)

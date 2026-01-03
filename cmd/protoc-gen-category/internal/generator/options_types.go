package generator

import (
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Extension field numbers
const (
	ExtFieldCategoryFile    = 50000
	ExtFieldCategory        = 50001
	ExtFieldFieldCategory   = 50002
	ExtFieldCategoryService = 50003
	ExtFieldCategoryMethod  = 50004
)

// parseExtension extracts raw bytes for an extension field from unknown fields
func parseExtension(m proto.Message, fieldNum protowire.Number) []byte {
	raw := m.ProtoReflect().GetUnknown()
	for len(raw) > 0 {
		num, typ, n := protowire.ConsumeTag(raw)
		if n < 0 {
			break
		}
		raw = raw[n:]

		var value []byte
		switch typ {
		case protowire.VarintType:
			_, n = protowire.ConsumeVarint(raw)
		case protowire.Fixed64Type:
			_, n = protowire.ConsumeFixed64(raw)
		case protowire.BytesType:
			value, n = protowire.ConsumeBytes(raw)
		case protowire.Fixed32Type:
			_, n = protowire.ConsumeFixed32(raw)
		default:
			return nil
		}
		if n < 0 {
			break
		}

		if num == fieldNum && typ == protowire.BytesType {
			return value
		}
		raw = raw[n:]
	}
	return nil
}

// CategoryFileOptions parsed from extension
type CategoryFileOptions struct {
	Effects []int32
	Stripe  *StripeConfig
}

// StripeConfig holds Stripe configuration
type StripeConfig struct {
	Plans            []string
	WebhookSecretEnv string
	ApiKeyEnv        string
}

func parseCategoryFileOptions(data []byte) *CategoryFileOptions {
	if data == nil {
		return nil
	}
	opts := &CategoryFileOptions{}
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			break
		}
		data = data[n:]

		switch typ {
		case protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				break
			}
			if num == 1 { // effects field
				opts.Effects = append(opts.Effects, int32(v))
			}
			data = data[n:]
		case protowire.BytesType:
			b, n := protowire.ConsumeBytes(data)
			if n < 0 {
				break
			}
			if num == 1 { // effects field (packed)
				for len(b) > 0 {
					v, vn := protowire.ConsumeVarint(b)
					if vn < 0 {
						break
					}
					opts.Effects = append(opts.Effects, int32(v))
					b = b[vn:]
				}
			} else if num == 10 { // stripe config
				opts.Stripe = parseStripeConfig(b)
			}
			data = data[n:]
		default:
			break
		}
	}
	return opts
}

func parseStripeConfig(data []byte) *StripeConfig {
	if data == nil {
		return nil
	}
	cfg := &StripeConfig{}
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			break
		}
		data = data[n:]

		if typ == protowire.BytesType {
			b, n := protowire.ConsumeBytes(data)
			if n < 0 {
				break
			}
			switch num {
			case 1: // plans (repeated string)
				cfg.Plans = append(cfg.Plans, string(b))
			case 2: // webhook_secret_env
				cfg.WebhookSecretEnv = string(b)
			case 3: // api_key_env
				cfg.ApiKeyEnv = string(b)
			}
			data = data[n:]
		} else {
			// skip other types
			if typ == protowire.VarintType {
				_, n = protowire.ConsumeVarint(data)
			} else if typ == protowire.Fixed64Type {
				n = 8
			} else if typ == protowire.Fixed32Type {
				n = 4
			}
			if n > 0 && n <= len(data) {
				data = data[n:]
			}
		}
	}
	return cfg
}

// CategoryMessageOptions parsed from extension
type CategoryMessageOptions struct {
	Functor            bool
	Monoid             bool
	Semigroup          bool
	Foldable           bool
	Traversable        bool
	Monad              bool
	Bifunctor          bool
	Combine            int32
	FirestoreBridge    bool
	Global             bool
	StripeCustomer     bool
	StripeSubscription bool
}

func parseCategoryMessageOptions(data []byte) *CategoryMessageOptions {
	if data == nil {
		return nil
	}
	opts := &CategoryMessageOptions{}
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			break
		}
		data = data[n:]

		if typ != protowire.VarintType {
			// Skip non-varint
			if typ == protowire.BytesType {
				_, n = protowire.ConsumeBytes(data)
			} else if typ == protowire.Fixed64Type {
				n = 8
			} else if typ == protowire.Fixed32Type {
				n = 4
			}
			if n > 0 && n <= len(data) {
				data = data[n:]
			}
			continue
		}

		v, n := protowire.ConsumeVarint(data)
		if n < 0 {
			break
		}
		data = data[n:]

		switch num {
		case 1:
			opts.Functor = v != 0
		case 2:
			opts.Monoid = v != 0
		case 3:
			opts.Semigroup = v != 0
		case 4:
			opts.Foldable = v != 0
		case 5:
			opts.Traversable = v != 0
		case 6:
			opts.Monad = v != 0
		case 7:
			opts.Bifunctor = v != 0
		case 8:
			opts.Combine = int32(v)
		case 10:
			opts.FirestoreBridge = v != 0
		case 11:
			opts.Global = v != 0
		case 20:
			opts.StripeCustomer = v != 0
		case 21:
			opts.StripeSubscription = v != 0
		}
	}
	return opts
}

// FieldCategoryOptions parsed from extension
type FieldCategoryOptions struct {
	Combine int32
	Empty   string
}

func parseFieldCategoryOptions(data []byte) *FieldCategoryOptions {
	if data == nil {
		return nil
	}
	opts := &FieldCategoryOptions{}
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			break
		}
		data = data[n:]

		switch typ {
		case protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				break
			}
			if num == 1 {
				opts.Combine = int32(v)
			}
			data = data[n:]
		case protowire.BytesType:
			b, n := protowire.ConsumeBytes(data)
			if n < 0 {
				break
			}
			if num == 2 {
				opts.Empty = string(b)
			}
			data = data[n:]
		default:
			break
		}
	}
	return opts
}

// CategoryServiceOptions parsed from extension
type CategoryServiceOptions struct {
	Kleisli        bool
	Middleware     bool
	Parallel       bool
	Retry          bool
	CircuitBreaker bool
	Fanout         bool
	Mock           bool
	ConnectBridge  bool
	StripeBilling  bool
}

func parseCategoryServiceOptions(data []byte) *CategoryServiceOptions {
	if data == nil {
		return nil
	}
	opts := &CategoryServiceOptions{}
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			break
		}
		data = data[n:]

		if typ != protowire.VarintType {
			if typ == protowire.BytesType {
				_, n = protowire.ConsumeBytes(data)
			} else if typ == protowire.Fixed64Type {
				n = 8
			} else if typ == protowire.Fixed32Type {
				n = 4
			}
			if n > 0 && n <= len(data) {
				data = data[n:]
			}
			continue
		}

		v, n := protowire.ConsumeVarint(data)
		if n < 0 {
			break
		}
		data = data[n:]

		switch num {
		case 1:
			opts.Kleisli = v != 0
		case 2:
			opts.Middleware = v != 0
		case 3:
			opts.Parallel = v != 0
		case 4:
			opts.Retry = v != 0
		case 5:
			opts.CircuitBreaker = v != 0
		case 6:
			opts.Fanout = v != 0
		case 7:
			opts.Mock = v != 0
		case 8:
			opts.ConnectBridge = v != 0
		case 10:
			opts.StripeBilling = v != 0
		}
	}
	return opts
}

// CategoryMethodOptions parsed from extension
type CategoryMethodOptions struct {
	Idempotent bool
	Fallback   string
	CacheKey   string
	MinPlan    string
	Metered    bool
	MeterEvent string
}

func parseCategoryMethodOptions(data []byte) *CategoryMethodOptions {
	if data == nil {
		return nil
	}
	opts := &CategoryMethodOptions{}
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			break
		}
		data = data[n:]

		switch typ {
		case protowire.VarintType:
			v, n := protowire.ConsumeVarint(data)
			if n < 0 {
				break
			}
			switch num {
			case 1:
				opts.Idempotent = v != 0
			case 11:
				opts.Metered = v != 0
			}
			data = data[n:]
		case protowire.BytesType:
			b, n := protowire.ConsumeBytes(data)
			if n < 0 {
				break
			}
			switch num {
			case 2:
				opts.Fallback = string(b)
			case 3:
				opts.CacheKey = string(b)
			case 10:
				opts.MinPlan = string(b)
			case 12:
				opts.MeterEvent = string(b)
			}
			data = data[n:]
		default:
			break
		}
	}
	return opts
}

// Dummy implementations for compatibility (E_* vars referenced in generator.go)
// These are placeholders - actual parsing uses the parseExtension + parse*Options functions
var (
	E_CategoryFile    = protoreflect.FieldNumber(ExtFieldCategoryFile)
	E_Category        = protoreflect.FieldNumber(ExtFieldCategory)
	E_FieldCategory   = protoreflect.FieldNumber(ExtFieldFieldCategory)
	E_CategoryService = protoreflect.FieldNumber(ExtFieldCategoryService)
	E_CategoryMethod  = protoreflect.FieldNumber(ExtFieldCategoryMethod)
)

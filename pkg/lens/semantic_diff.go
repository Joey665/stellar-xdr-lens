package lens

import (
	"fmt"
	"math/big"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// DiffSemantic compares two decoded values using type-aware representations
// where the wire structure obscures a value's meaning. ScVal wide integers are
// normalized to exact decimal integers; existing enrichers provide canonical
// asset and address identifiers. Unsupported types retain structural behavior.
func DiffSemantic(left, right *Value) (*DiffResult, error) {
	return DiffWithOptions(left, right, DiffOptions{Semantic: true})
}

// SemanticMemoComparison evaluates whether two Memo variants carry equivalent payload
// (e.g. MemoText "100" vs MemoId 100) and produces an informative semantic note (#127).
func SemanticMemoComparison(left, right xdr.Memo) (equivalent bool, note string) {
	if left.Type == xdr.MemoTypeMemoText && right.Type == xdr.MemoTypeMemoId {
		if left.Text != nil && right.Id != nil && fmt.Sprintf("%d", *right.Id) == *left.Text {
			return true, "memo content equivalent: MemoText matches MemoId numeric string"
		}
	}
	if left.Type == xdr.MemoTypeMemoId && right.Type == xdr.MemoTypeMemoText {
		if right.Text != nil && left.Id != nil && fmt.Sprintf("%d", *left.Id) == *right.Text {
			return true, "memo content equivalent: MemoId matches MemoText numeric string"
		}
	}
	return false, ""
}

func semanticScValInteger(value xdr.ScVal) (string, bool) {
	switch value.Type {
	case xdr.ScValTypeScvU128:
		if value.U128 == nil {
			return "", false
		}
		parts := value.U128
		return unsignedLimbs(uint64(parts.Hi), uint64(parts.Lo)).String(), true
	case xdr.ScValTypeScvI128:
		if value.I128 == nil {
			return "", false
		}
		parts := value.I128
		return signedLimbs(int64(parts.Hi), uint64(parts.Lo)).String(), true
	case xdr.ScValTypeScvU256:
		if value.U256 == nil {
			return "", false
		}
		parts := value.U256
		return unsignedLimbs(uint64(parts.HiHi), uint64(parts.HiLo), uint64(parts.LoHi), uint64(parts.LoLo)).String(), true
	case xdr.ScValTypeScvI256:
		if value.I256 == nil {
			return "", false
		}
		parts := value.I256
		return signedLimbs(int64(parts.HiHi), uint64(parts.HiLo), uint64(parts.LoHi), uint64(parts.LoLo)).String(), true
	default:
		return "", false
	}
}

func unsignedLimbs(limbs ...uint64) *big.Int {
	value := new(big.Int)
	for _, limb := range limbs {
		value.Lsh(value, 64)
		value.Or(value, new(big.Int).SetUint64(limb))
	}
	return value
}

func signedLimbs(high int64, remaining ...uint64) *big.Int {
	value := big.NewInt(high)
	for _, limb := range remaining {
		value.Lsh(value, 64)
		value.Add(value, new(big.Int).SetUint64(limb))
	}
	return value
}

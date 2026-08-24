// Package evidence models the immutable cold-chain and physicochemical
// measurement ledgers, plus the fixed-point arithmetic used to derive them.
package evidence

import "github.com/dairygate/raw-milk-tank-intake-inspection/catalog"

// EvidenceType discriminates the measurement kinds.
type EvidenceType string

const (
	EvidenceAntibiotic    EvidenceType = "antibiotic"     // 抗生素抑制圈
	EvidenceSomaticCell   EvidenceType = "somatic_cell"   // 体细胞
	EvidenceColony        EvidenceType = "colony"         // 菌落
	EvidenceFreezingPoint EvidenceType = "freezing_point" // 冰点
	EvidenceFat           EvidenceType = "fat"            // 脂肪
	EvidenceProtein       EvidenceType = "protein"        // 蛋白
	EvidenceTemperature   EvidenceType = "temperature"    // 冷链温度
)

// EvidenceRecord is an immutable, generation-stamped measurement. Once
// validly written it can never be overwritten, only superseded by a newer
// generation rejudgement.
type EvidenceRecord struct {
	TaskID      string                  `json:"taskId"`
	BlindCode   string                  `json:"blindCode,omitempty"`
	Compartment catalog.CompartmentCode `json:"compartment,omitempty"`
	Well        string                  `json:"well,omitempty"`
	Type        EvidenceType            `json:"type"`
	Raw         FixedPoint              `json:"raw"`
	Derived     *FixedPoint             `json:"derived,omitempty"`
	RuleVersion string                  `json:"ruleVersion"`
	Generation  int64                   `json:"generation"`
	Immutable   bool                    `json:"immutable"`
}

// TemperatureCell is a single locked-window temperature sample.
type TemperatureCell struct {
	TaskID     string     `json:"taskId"`
	RecorderID string     `json:"recorderId"`
	AtSeconds  int64      `json:"atSeconds"`
	Celsius    FixedPoint `json:"celsius"`
	WindowSeq  int        `json:"windowSeq"`
	Covered    bool       `json:"covered"`
}

// Writer is the immutable evidence sink. Every valid write is append-only;
// failed writes leave no partial evidence behind.
type Writer interface {
	// Write validates then appends one evidence record.
	Write(r EvidenceRecord) error
	// WriteTemperature validates then appends one temperature cell batch.
	WriteTemperature(cells []TemperatureCell) error
}

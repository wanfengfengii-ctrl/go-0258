package inspection

// Status is the lifecycle state of an inspection task. The progression is
// strictly ordered and only moves forward; terminal states never regress.
type Status string

const (
	StatusPendingBuild       Status = "pending_build"        // 待建检
	StatusPendingSampling    Status = "pending_sampling"     // 待采样确认
	StatusBlindSplitting     Status = "blind_splitting"      // 盲码分管中
	StatusPlateOccupied      Status = "plate_occupied"       // 板孔占用中
	StatusColdChainVerifying Status = "cold_chain_verifying" // 冷链核验中
	StatusAntibioticReading  Status = "antibiotic_reading"   // 抗生素读数中
	StatusMicrobialCulturing Status = "microbial_culturing"  // 微生物培养中
	StatusPhysicochemical    Status = "physicochemical"      // 理化复测中
	StatusPendingReview      Status = "pending_review"       // 待独立复核
	StatusAdmissible         Status = "admissible"           // 可入厂
	StatusEntered            Status = "entered"              // 已入厂
	StatusQuarantined        Status = "quarantined"          // 质量隔离
	StatusCancelled          Status = "cancelled"            // 已取消
)

// order maps each non-terminal status to the single status that may follow it.
var order = map[Status]Status{
	StatusPendingBuild:       StatusPendingSampling,
	StatusPendingSampling:    StatusBlindSplitting,
	StatusBlindSplitting:     StatusPlateOccupied,
	StatusPlateOccupied:      StatusColdChainVerifying,
	StatusColdChainVerifying: StatusAntibioticReading,
	StatusAntibioticReading:  StatusMicrobialCulturing,
	StatusMicrobialCulturing: StatusPhysicochemical,
	StatusPhysicochemical:    StatusPendingReview,
}

// terminal reports whether s is one of the four final outcomes.
func (s Status) terminal() bool {
	switch s {
	case StatusAdmissible, StatusEntered, StatusQuarantined, StatusCancelled:
		return true
	default:
		return false
	}
}

// IsTerminal reports whether s is a final outcome.
func (s Status) IsTerminal() bool { return s.terminal() }

// CanAdvanceTo reports whether moving from s to next is a legal transition.
// Forward movement follows the ordered pipeline; reaching any of the four
// terminal outcomes is allowed only from StatusPendingReview, and terminal
// states never advance further.
func (s Status) CanAdvanceTo(next Status) bool {
	if s.terminal() {
		return false
	}
	if next.terminal() {
		return s == StatusPendingReview
	}
	return order[s] == next
}

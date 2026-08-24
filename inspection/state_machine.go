package inspection

// Command identifies the business operation a caller is attempting to run, so
// the state machine can gate it against the task's current status.
type Command string

const (
	CmdSamplingConfirm   Command = "sampling_confirm"
	CmdBlindSplit        Command = "blind_split"
	CmdOccupancyAcquire  Command = "occupancy_acquire"
	CmdColdChainReadings Command = "cold_chain_readings"
	CmdAntibioticReading Command = "antibiotic_reading"
	CmdMicrobialReading  Command = "microbial_reading"
	CmdPhysicoReading    Command = "physicochemical_reading"
	CmdRejudge           Command = "rejudge"
	CmdReview            Command = "review"
	CmdFinalize          Command = "finalize"
)

// allowedCommands maps each non-terminal status to the commands it admits.
// A command is admitted only at the status where it is semantically valid;
// every other status yields a deterministic rejection.
var allowedCommands = map[Status][]Command{
	StatusPendingSampling:    {CmdSamplingConfirm},
	StatusBlindSplitting:     {CmdBlindSplit},
	StatusPlateOccupied:      {CmdOccupancyAcquire},
	StatusColdChainVerifying: {CmdColdChainReadings},
	StatusAntibioticReading:  {CmdAntibioticReading},
	StatusMicrobialCulturing: {CmdMicrobialReading},
	StatusPhysicochemical:    {CmdPhysicoReading},
	StatusPendingReview:      {CmdRejudge, CmdReview, CmdFinalize},
}

// Allows reports whether the command may execute at the given status.
func Allows(status Status, cmd Command) bool {
	for _, c := range allowedCommands[status] {
		if c == cmd {
			return true
		}
	}
	return false
}

// NextAfter returns the status that follows status after cmd completes, or
// false when cmd does not advance the task at status.
func NextAfter(status Status, cmd Command) (Status, bool) {
	if !Allows(status, cmd) {
		return status, false
	}
	switch status {
	case StatusPendingSampling:
		return StatusBlindSplitting, true
	case StatusBlindSplitting:
		return StatusPlateOccupied, true
	case StatusPlateOccupied:
		return StatusColdChainVerifying, true
	case StatusColdChainVerifying:
		return StatusAntibioticReading, true
	case StatusAntibioticReading:
		return StatusMicrobialCulturing, true
	case StatusMicrobialCulturing:
		return StatusPhysicochemical, true
	case StatusPhysicochemical:
		return StatusPendingReview, true
	default:
		return status, false
	}
}

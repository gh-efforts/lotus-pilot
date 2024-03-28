package build

const BuildVersion = "0.0.2"

var CurrentCommit string
var BuildType = "+mainnet"

func UserVersion() string {
	return BuildVersion + BuildType + CurrentCommit
}

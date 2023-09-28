package build

const BuildVersion = "0.0.3"

var CurrentCommit string

func UserVersion() string {
	return BuildVersion + CurrentCommit
}

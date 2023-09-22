package build

const BuildVersion = "0.0.2"

var CurrentCommit string

func UserVersion() string {
	return BuildVersion + CurrentCommit
}

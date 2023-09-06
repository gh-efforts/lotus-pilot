package main

const BuildVersion = "0.0.1"

var CurrentCommit string

func UserVersion() string {
	return BuildVersion + CurrentCommit
}

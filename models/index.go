package models

import "github.com/cherrai/nyanyago-utils/nlog"

var (
	log = nlog.New()
)

func InitModelIndex() {
	new(Trip).CreateIndex()
}

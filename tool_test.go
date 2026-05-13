package tool_box

import (
	"testing"

	"github.com/jiajia556/tool-box/log"
	_ "github.com/jiajia556/tool-box/log/std"
)

func TestLog(t *testing.T) {
	err := log.Init("std", log.Config{})
	if err != nil {
		t.Fatal(err)
	}

	log.Debug("test log", "key1", "value1", "key2", "value2")
}

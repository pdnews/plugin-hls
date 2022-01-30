package hls

import (
	"github.com/gogf/gf/test/gtest"
	"testing"
	"time"
)

// TestTsFileName 校验时间格式生成
func TestTsFileName(t *testing.T) {
	var cst = time.FixedZone("CST", 8*3600)
	now := time.Unix(1577970855, 123000000).In(cst)

	var template = map[string]interface{}{
		"[2006]":      now.Format("2006"),
		"[01]":        now.Format("01"),
		"[02]":        now.Format("02"),
		"[15]":        now.Format("15"),
		"[04]":        now.Format("04"),
		"[05]":        now.Format("05"),
		"[999]":       now.UnixMilli() - now.Unix()*1e3,
		"[timestamp]": now.UnixMilli(),
		"[second]":    now.Unix(),
	}

	var match = map[string]interface{}{
		"[01]":        "01",
		"[02]":        "02",
		"[04]":        "14",
		"[05]":        "15",
		"[15]":        "21",
		"[2006]":      "2020",
		"[999]":       123,
		"[seq]":       0,
		"[timestamp]": 1577970855123,
		"[second]":    1577970855,
	}

	for k, v := range template {
		gtest.Assert(match[k], v)
	}
}

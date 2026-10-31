package main

import "testing"

func TestEffectiveCNSplit(t *testing.T) {
	orig := getSetting("cn_split", "")
	defer setSetting("cn_split", orig)

	cases := []struct {
		name  string
		split string
		h     realmCount
		want  int
	}{
		{"未知（hub 没查到）用配置值", "70", realmCount{}, 70},
		{"两边都健康 → 按号数配比 13:2", "70", realmCount{CN: 13, Intl: 2, Known: true}, 86},
		{"intl 全灭 → 全走 cn", "70", realmCount{CN: 13, Intl: 0, Known: true}, 100},
		{"cn 全灭 → 全走 intl", "70", realmCount{CN: 0, Intl: 2, Known: true}, 0},
		{"号数对等 → 配置 70 生效", "70", realmCount{CN: 5, Intl: 5, Known: true}, 70},
		{"容量比配置更低时以容量的 cn 占比为准", "30", realmCount{CN: 9, Intl: 1, Known: true}, 90},
		{"强制 100", "100", realmCount{CN: 5, Intl: 5, Known: true}, 100},
		{"强制 0", "0", realmCount{CN: 5, Intl: 5, Known: true}, 0},
	}
	for _, c := range cases {
		setSetting("cn_split", c.split)
		if got := effectiveCNSplitPercentWith(c.h); got != c.want {
			t.Errorf("%s：got %d, want %d", c.name, got, c.want)
		}
	}
}

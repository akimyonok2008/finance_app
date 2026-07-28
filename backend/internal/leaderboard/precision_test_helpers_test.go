package leaderboard

import "github.com/ardakimyonok/finance_app/internal/money"

func testIndex(value string) money.IndexValue { return money.MustIndexValue(value) }
func testRatio(value string) money.Ratio      { return money.MustRatio(value) }

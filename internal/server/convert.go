package server

import "github.com/abjelosevic88/pocket-homelab-travel/internal/currency"

func convert(amount int64, from, to string, rate float64) int64 {
	return currency.Convert(amount, from, to, rate)
}

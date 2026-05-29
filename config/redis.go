package conf

import (
	"time"

	"github.com/cherrai/nyanyago-utils/nredis"
)

var Redisdb *nredis.NRedis

var BaseKey = "meow-whisper"

var RedisCacheKeys = map[string]*nredis.RedisCacheKeysType{
	"GetTrip": {
		Key:        "GetTrip",
		Expiration: 5 * 60 * time.Second,
	},
	"GetVehicle": {
		Key:        "GetVehicle",
		Expiration: 5 * 60 * time.Second,
	},
	"GetVehicles": {
		Key:        "GetVehicles",
		Expiration: 5 * 60 * time.Second,
	},
	"GetTripPositions": {
		Key:        "GetTripPositions",
		Expiration: 5 * 60 * time.Second,
	},
	"GetTripByShareKey": {
		Key:        "GetTripByShareKey",
		Expiration: 5 * 60 * time.Second,
	},
	"GetTrips": {
		Key:        "GetTrips",
		Expiration: 5 * 60 * time.Second,
	},
	"GetAllTripPositions": {
		Key:        "GetAllTripPositions",
		Expiration: 5 * 60 * time.Second,
	},
	"GetUserPosition": {
		Key:        "GetUserPosition",
		Expiration: 5 * 60 * time.Second,
	},
	"GetCity": {
		Key:        "GetCity",
		Expiration: 5 * 60 * time.Second,
	},
	"GetJM": {
		Key:        "GetJM",
		Expiration: 5 * 60 * time.Second,
	},
	"GetCities": {
		Key:        "GetCities",
		Expiration: 5 * 60 * time.Second,
	},
	"Test": {
		Key:        "Test",
		Expiration: 5 * 60 * time.Second,
	},
	"GetOsmInfo": {
		Key:        "GetOsmInfo",
		Expiration: 5 * 60 * time.Second,
	},
}

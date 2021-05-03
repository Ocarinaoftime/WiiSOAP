//	Copyright (C) 2018-2020 CornierKhan1
//
//	WiiSOAP is SOAP Server Software, designed specifically to handle Wii Shop Channel SOAP.
//
//    This program is free software: you can redistribute it and/or modify
//    it under the terms of the GNU Affero General Public License as published
//    by the Free Software Foundation, either version 3 of the License, or
//    (at your option) any later version.
//
//    This program is distributed in the hope that it will be useful,
//    but WITHOUT ANY WARRANTY; without even the implied warranty of
//    MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//    GNU Affero General Public License for more details.
//
//    You should have received a copy of the GNU Affero General Public License
//    along with this program.  If not, see http://www.gnu.org/licenses/.

package main

import (
	"crypto/md5"
	"errors"
	"fmt"
	wiino "github.com/RiiConnect24/wiino/golang"
	"github.com/jackc/pgconn"
	"log"
	"math/rand"
	"strconv"
)

const (
	PrepareUserStatement = `INSERT INTO userbase (device_id, device_token, device_token_hashed, account_id, region, country, language, serial_number, device_code)  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	SyncUserStatement    = `SELECT account_id, device_code, device_token FROM userbase WHERE language = $1 AND country = $2 AND region = $3 AND device_id = $4`
)

func checkRegistration(e *Envelope) {
	serialNo, err := getKey(e.doc, "SerialNumber")
	if err != nil {
		e.Error(5, "not good enough for me. ;3", err)
		return
	}

	e.AddKVNode("OriginalSerialNumber", serialNo)
	e.AddKVNode("DeviceStatus", "R")
}

func getChallenge(e *Envelope) {
	// The official Wii Shop Channel requests a Challenge from the server, and promptly disregards it.
	// (Sometimes, it may not request a challenge at all.) No attempt is made to validate the response.
	// It then uses another hard-coded value in place of this returned value entirely in any situation.
	// For this reason, we consider it irrelevant.
	e.AddKVNode("Challenge", SharedChallenge)
}

func getRegistrationInfo(e *Envelope) {
	// GetRegistrationInfo is SyncRegistration with authentication and an additional key.
	syncRegistration(e)

	// This _must_ be POINTS.
	// It does not appear to be observed by any known client,
	// but is sent by Nintendo in official requests.
	e.AddKVNode("Currency", "POINTS")
}

func syncRegistration(e *Envelope) {
	var accountId int64
	var deviceCode int
	var deviceToken string

	user := pool.QueryRow(ctx, SyncUserStatement, e.Language(), e.Country(), e.Region(), e.DeviceId())
	err := user.Scan(&accountId, &deviceCode, &deviceToken)
	if err != nil {
		e.Error(7, "An error occurred querying the database.", err)
	}

	e.AddKVNode("AccountId", strconv.FormatInt(accountId, 10))
	e.AddKVNode("DeviceToken", deviceToken)
	e.AddKVNode("DeviceTokenExpired", "false")
	e.AddKVNode("Country", e.Country())
	e.AddKVNode("ExtAccountId", "")
	e.AddKVNode("DeviceStatus", "R")
}

func register(e *Envelope) {
	reason := "disgustingly invalid. ;3"
	deviceCode, err := getKey(e.doc, "DeviceCode")
	if err != nil {
		e.Error(7, reason, err)
		return
	}

	registerRegion, err := getKey(e.doc, "RegisterRegion")
	if err != nil {
		e.Error(7, reason, err)
		return
	}
	if registerRegion != e.Region() {
		e.Error(7, reason, errors.New("region does not match registration region"))
		return
	}

	serialNo, err := getKey(e.doc, "SerialNumber")
	if err != nil {
		e.Error(7, reason, err)
		return
	}

	// Validate given friend code.
	userId, err := strconv.ParseUint(deviceCode, 10, 64)
	if err != nil {
		e.Error(7, reason, err)
		return
	}
	if wiino.NWC24CheckUserID(userId) != 0 {
		e.Error(7, reason, err)
		return
	}

	// Generate a random 9-digit number, padding zeros as necessary.
	accountId := rand.Int63n(999999999)

	// Generate a device token, 21 characters...
	deviceToken := RandString(21)
	// ...and then its md5, because the Wii sends this for most requests.
	md5DeviceToken := fmt.Sprintf("%x", md5.Sum([]byte(deviceToken)))

	// Insert all of our obtained values to the database..
	_, err = pool.Exec(ctx, PrepareUserStatement, e.DeviceId(), deviceToken, md5DeviceToken, accountId, e.Region(), e.Country(), e.Language(), serialNo, deviceCode)
	if err != nil {
		// It's okay if this isn't a PostgreSQL error, as perhaps other issues have come in.
		if driverErr, ok := err.(*pgconn.PgError); ok {
			if driverErr.Code == "23505" {
				e.Error(7, reason, errors.New("user already exists"))
				return
			}
		}
		log.Printf("error executing statement: %v\n", err)
		e.Error(7, reason, errors.New("failed to execute db operation"))
		return
	}

	fmt.Println("The request is valid! Responding...")
	e.AddKVNode("AccountId", strconv.FormatInt(accountId, 10))
	e.AddKVNode("DeviceToken", deviceToken)
	e.AddKVNode("DeviceTokenExpired", "false")
	e.AddKVNode("Country", e.Country())
	// Optionally, one can send back DeviceCode and ExtAccountId to update on device.
	// We send these back as-is regardless.
	e.AddKVNode("ExtAccountId", "")
	e.AddKVNode("DeviceCode", deviceCode)
}

func unregister(e *Envelope) {
	// how abnormal... ;3
}

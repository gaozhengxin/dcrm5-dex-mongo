// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package core

// Constants containing the genesis allocation of built-in genesis blocks.
// Their content is an RLP-encoded list of (address, balance) tuples.
// Use mkalloc.go to create/update them.
const FsnchainAllocJson = `{
	"0x3a1b3b81ed061581558a81f11d63e03129347437": {"balance": "10240000000000000000000000"},
	"0x0963a18ea497b7724340fdfe4ff6e060d3f9e388": {"balance": "10240000000000000000000000"},
	"0x33ee8b7a872701794afca3357c1cfc1791865541": {"balance": "10240000000000000000000000"}
}`

const FsnchainETHTestAllocJson = `{
	"0xe16ef4a8f221ea9b81c30e3a84d6a5c50c2befb2": {"balance": "10240000000000000000000000"},
	"0xe2c50f6f9039db18a61634b032345274475f033d": {"balance": "10240000000000000000000000"},
	"0xe309c1f41d87b6edbaf8b85121e9190ca6278bea": {"balance": "10240000000000000000000000"}
}`

const FsnchainBTCTestAllocJson = `{
	"mfeLSqhm8i18h3mw6bqySV7CfMbmX4nUmN": {"balance": "1024000000000000"},
	"mmwr9EkM5h7g7jqYYYDwVNF7NXrKQ8VUE2": {"balance": "1024000000000000"},
	"mztyuHWEMLw4BqXwiJF1kv2HUt9TW34N3L": {"balance": "1024000000000000"}
}`


const FsnchainAllocJson2 = `{
    "ODBS":[
    {"ACCOUNT":"0x3a1b3b81ed061581558a81f11d63e03129347437","BALANCE":"10240000000000000000000000"},
    {"ACCOUNT":"0x0963a18ea497b7724340fdfe4ff6e060d3f9e388","BALANCE":"10240000000000000000000000"},
    {"ACCOUNT":"0x33ee8b7a872701794afca3357c1cfc1791865541","BALANCE":"10240000000000000000000000"}
    ]
}`

const FsnchainETHTestAllocJson2 = `{
    "ODBS":[
    {"ACCOUNT":"0xe16ef4a8f221ea9b81c30e3a84d6a5c50c2befb2","BALANCE":"10240000000000000000000000"},
    {"ACCOUNT":"0xe2c50f6f9039db18a61634b032345274475f033d","BALANCE":"10240000000000000000000000"},
    {"ACCOUNT":"0xe309c1f41d87b6edbaf8b85121e9190ca6278bea","BALANCE":"10240000000000000000000000"}
    ]
}`

const FsnchainBTCTestAllocJson2 = `{
    "ODBS":[
    {"ACCOUNT":"mfeLSqhm8i18h3mw6bqySV7CfMbmX4nUmN","BALANCE":"1024000000000000"},
    {"ACCOUNT":"mmwr9EkM5h7g7jqYYYDwVNF7NXrKQ8VUE2","BALANCE":"1024000000000000"},
    {"ACCOUNT":"mztyuHWEMLw4BqXwiJF1kv2HUt9TW34N3L","BALANCE":"1024000000000000"}
    ]
}`

const fsnchainPPOWTestAllocJson = `{
}`

const fsnchainPPOWDevAllocJson = `{
}`

// nolint: misspell
const mainnetAllocData = ""
const testnetAllocData = ""
const rinkebyAllocData = ""


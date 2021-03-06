#!/bin/sh
#
# Copyright (c) 2014 Christian Couder
# MIT Licensed; see the LICENSE file in this repository.
#

test_description="Test add and cat commands"

. lib/test-lib.sh

test_launch_ipfs_mount

test_expect_success "ipfs add succeeds" '
	echo "Hello Worlds!" >mountdir/hello.txt &&
	ipfs add mountdir/hello.txt >actual
'

test_expect_success "ipfs add output looks good" '
	HASH="QmVr26fY1tKyspEJBniVhqxQeEjhF78XerGiqWAwraVLQH" &&
	echo "added $HASH $(pwd)/mountdir/hello.txt" >expected &&
	test_cmp expected actual
'

test_expect_success "ipfs cat succeeds" '
	ipfs cat $HASH >actual
'

test_expect_success "ipfs cat output looks good" '
	echo "Hello Worlds!" >expected &&
	test_cmp expected actual
'

test_expect_success FUSE "cat ipfs/stuff succeeds" '
	cat ipfs/$HASH >actual
'

test_expect_success FUSE "cat ipfs/stuff looks good" '
	test_cmp expected actual
'

test_expect_success "go-random is installed" '
	type random
'

test_expect_success "generate 100MB file using go-random" '
	random 104857600 42 >mountdir/bigfile
'

test_expect_success "sha1 of the file looks ok" '
	echo "ae986dd159e4f014aee7409cdc2001ea74f618d1  mountdir/bigfile" >sha1_expected &&
	shasum mountdir/bigfile >sha1_actual &&
	test_cmp sha1_expected sha1_actual
'

test_expect_success "ipfs add bigfile succeeds" '
	ipfs add mountdir/bigfile >actual
'

test_expect_success "ipfs add bigfile output looks good" '
	HASH="QmVm3Da371opC3hpsCLuYSozdyM6wRvu9UoUqoyW8u4LRq" &&
	echo "added $HASH $(pwd)/mountdir/bigfile" >expected &&
	test_cmp expected actual
'

test_expect_success "ipfs cat succeeds" '
	ipfs cat $HASH | shasum >sha1_actual
'

test_expect_success "ipfs cat output looks good" '
	echo "ae986dd159e4f014aee7409cdc2001ea74f618d1  -" >sha1_expected &&
	test_cmp sha1_expected sha1_actual
'

test_expect_success FUSE "cat ipfs/bigfile succeeds" '
	cat ipfs/$HASH | shasum >sha1_actual
'

test_expect_success FUSE "cat ipfs/bigfile looks good" '
	test_cmp sha1_expected sha1_actual
'

test_kill_ipfs_mount

test_done

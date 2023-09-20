#!/bin/bash

base_dir=/media/nvme/t0101
mkdir -p $base_dir

repo_dir=$base_dir/.lotusworker
log_dir=$base_dir/log
tmp_dir=$base_dir/tmp
parent_cache_dir=$base_dir/parent_cache

mkdir -p $repo_dir $log_dir $tmp_dir $parent_cache_dir
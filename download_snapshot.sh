#!/bin/bash
# Written by muXxer

timestamp_file="$1.timestamp"
snapshot_file="$1.snap"

declare -a snapshot_urls=("http://field.carriota.com:8088/" "http://db.tangletools.org/hercules/")

latest_timestamp=0
for snapshot_url in ${snapshot_urls[@]}; do
    printf "\nSearching latest snapshot at "${snapshot_url}"...\n"
    lastest_file_name=`curl -s ${snapshot_url} | grep -o 'href="[0-9]\+.snap"' | cut -d '"' -f 2 | tail -1`
    lastest_file_name_wo_ext=$((`echo ${lastest_file_name} | cut -d '.' -f 1`))
    printf "   "${lastest_file_name}"\n"
    if [[ $latest_timestamp -lt $lastest_file_name_wo_ext ]] ; then
        latest_timestamp=${lastest_file_name_wo_ext}
        latest_file=$snapshot_url$lastest_file_name
    fi
done

existing_timestamp=0
if [ -e ${timestamp_file} ] && [ -e ${snapshot_file} ] ; then
    existing_timestamp=$((`cat ${timestamp_file}`))
    if [[ $latest_timestamp -gt $existing_timestamp ]] ; then
        rm ${timestamp_file}
    fi
fi

if [[ $latest_timestamp -gt $existing_timestamp ]] ; then
    printf "\nLatest snapshot: "$latest_file"\n\n"
    curl -L $latest_file -o ${snapshot_file}
    echo "${latest_timestamp}" > ${timestamp_file}
else
    printf "\nSnapshot already up to date: "$existing_timestamp"\n"
fi

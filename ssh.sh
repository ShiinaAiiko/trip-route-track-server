#! /bin/bash

DIR=$(cd $(dirname $0) && pwd)
allowMethods=("run")

host=$BUILD_SERVER_HOST
user=$BUILD_SERVER_USER
password=$BUILD_SERVER_PASSWORD
projectPath=$BUILD_SERVER_PROJECT_ROOTP_PATH

run() {
	echo "-> 正在传输编译后的文件至服务器"

	local files=(
		"./trip"
		"release.sh"
		"config.pro.json"
		"Dockerfile.multi"
		"Dockerfile.run.multi"
	)

	sshpass -p $password \
		rsync -avz \
		"${files[@]}" \
		$user@$host:$projectPath/nyanya-trip-route-track/server/

	echo "-> 传输完毕"
	echo "-> 正在执行相关命令"
	sshpass -p $password ssh $user@$host <<bash
    cd $projectPath/nyanya-trip-route-track/server
    ./release.sh run
bash
	echo "-> 执行完毕"
}

main() {
	if echo "${allowMethods[@]}" | grep -wq "$1"; then
		"$1"
	else
		echo "Invalid command: $1"
	fi
}

main "$1"

#! /bin/bash
name="nyanya-trip-route-track-server"
runName="$name-run"
port=23203
branch="main"
# configFilePath="config.dev.json"
configFilePath="config.pro.json"
DIR=$(cd $(dirname $0) && pwd)
allowMethods=("unzip backup runexec run stop gitpull protos dockerremove start logs")


gitpull() {
	echo "-> 正在拉取远程仓库"
	git reset --hard
	git pull origin $branch
}

runexec() {
	docker exec -it $runName /bin/sh
}

dockerremove() {
	echo "-> 删除无用镜像"
	docker rm $(docker ps -q -f status=exited) 2 &>/dev/null
	docker rmi -f $(docker images | grep '<none>' | awk '{print $3}') 2 &>/dev/null
}

start() {
	# ./client/release.sh start && 。/release.sh start

	echo "-> 正在启动「${name}」服务"
	# gitpull
	dockerremove

	echo "-> 正在准备相关资源"
	# cp -r ../protos $DIR/protos_temp
	cp -r ~/.ssh $DIR
	cp -r ~/.gitconfig $DIR

	cp -r ../protos $DIR/protos_temp

	mkdir -p $DIR/static
	mkdir -p $DIR/fsdb

	echo "-> 准备构建Docker"

	docker build \
		-t \
		$name \
		--network host \
		$(cat /etc/hosts | sed 's/^#.*//g' | grep '[0-9][0-9]' | tr "\t" " " | awk '{print "--add-host="$2":"$1 }' | tr '\n' ' ') \
		. \
		-f Dockerfile.multi
	rm -rf $DIR/.ssh
	rm -rf $DIR/.gitconfig
	rm -rf $DIR/protos_temp

	echo "-> 准备运行Docker"
	stop
	# -v $DIR/fsdb:/fsdb \
	docker run \
		-v $DIR/$configFilePath:/config.json \
		-v $DIR/appList.json:/appList.json \
		-v $DIR/client:/client \
		-v $DIR/static:/static \
		-v $DIR/fsdb:/fsdb \
		-v /etc/timezone:/etc/timezone:ro \
		-v /etc/localtime:/etc/localtime:ro \
    --add-host=host.docker.internal:host-gateway \
		--name=$name \
		$(cat /etc/hosts | sed 's/^#.*//g' | grep '[0-9][0-9]' | tr "\t" " " | awk '{print "--add-host="$2":"$1 }' | tr '\n' ' ') \
		-p $port:$port \
		--restart=always \
		-d $name

	echo "-> 整理文件资源"
	docker cp $name:/trip $DIR/trip
	stop

	./ssh.sh run

	rm -rf $DIR/trip
}

backup() {
	# backupTime=$(date +'%Y-%m-%d_%T')
	# zip -q -r ./saass_$backupTime.zip ./static
	tar cvzf $DIR/trip_static.tgz -C $DIR/static .

	# unzip -d ./ build_2023-07-04_21:11:13.zip
}

unzip() {
	mkdir -p $DIR/static
	tar xvzf $DIR/trip_static.tgz -C $DIR/static
}

run() {
	echo "-> 正在启动「${runName}」服务"
	dockerremove

	mkdir -p $DIR/static
	mkdir -p $DIR/fsdb

	echo "-> 准备构建Docker"


	docker build \
		-t \
		$runName \
		--network host \
		. \
		-f Dockerfile.run.multi

	echo "-> 准备运行Docker"
	stop
	# -v $DIR/fsdb:/fsdb \
	docker run \
		-v $DIR/$configFilePath:/config.json \
		-v $DIR/appList.json:/appList.json \
		-v $DIR/client:/client \
		-v $DIR/static:/static \
		-v $DIR/fsdb:/fsdb \
		-v /etc/timezone:/etc/timezone:ro \
		-v /etc/localtime:/etc/localtime:ro \
    --add-host=host.docker.internal:host-gateway \
		--name=$runName \
		-p $port:$port \
		--restart=always \
		-d $runName
}

stop() {
	docker stop $name
	docker rm $name
	docker stop $runName
	docker rm $runName
}

protos() {
	echo "-> 准备编译Protobuf"
	# cp -r ../protos $DIR/protos_temp
	# cd ./protos && protoc --go_out=. *.proto
	mkdir -p ./protos
	cp -r ../protos/* ./protos/
	cd ./protos
	protoc --go_out=../protos *.proto
	cd ..
	# cd ../protos && protoc  --go_out=../server/protos *.proto

	rm -f ./protos/*.proto
	# protoc --go_out=./protos --proto_path=../protos/**/*.proto

	echo "-> 编译Protobuf成功"
}

logs() {
	docker logs -f $runName
}

main() {
	if echo "${allowMethods[@]}" | grep -wq "$1"; then
		"$1"
	else
		echo "Invalid command: $1"
	fi
}

main "$1"

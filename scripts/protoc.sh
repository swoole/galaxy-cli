cPath=$(cd $(dirname $0);pwd)
rootPath=$(cd "$cPath"/../;pwd)
goOut=$rootPath
protoPath="$rootPath"/proto/*.proto
protoc "--go_out=$goOut"  $protoPath -I "$rootPath"/proto -I "/usr/local/include/"
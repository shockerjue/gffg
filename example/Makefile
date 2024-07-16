default: build

build:
	make -C server

	rm -rf bin
	mkdir bin
	mkdir bin/client
	mkdir bin/client/conf
	mkdir bin/server
	cp -rf ./client/gffg.xml bin/client/conf/
	cp ./server/client bin/client/
	cp -rf ./server/conf bin/server/
	cp ./server/server bin/server/
	rm ./server/client
	rm ./server/server

clean:
	rm -rf bin

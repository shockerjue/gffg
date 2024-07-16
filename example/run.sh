#!/bin/bash
PWD=$(pwd)
echo "cd  ${PWD}"

# Operation Service Management Center
polaris() {
    docker run -d --privileged=true \
    -p 15010:15010 \
    -p 8101:8101 \
    -p 8100:8100 \
    -p 8080:8080 \
    -p 8090:8090 \
    -p 8091:8091 \
    -p 8093:8093 \
    -p 8761:8761 \
    -p 8848:8848 \
    -p 9848:9848 \
    -p 9090:9090 \
    -p 9091:9091 polarismesh/polaris-standalone:latest
}

# Run the grafana service to view monitoring information
grafna() {
    docker run -d --name=grafana -p 3000:3000 grafana/grafana-enterprise
}

# Start server
server() {
    SVR_DIR=$(PWD)/bin/server
    echo "cd ${SVR_DIR}"
    echo "start server ========> "
    nohup ./server > /dev/null 2>&1 &
    echo "server open success ========> "
}


echo "Start the Service Management Center service =======>"
polaris
sleep 1
echo "Start the service management center service completed =====>"

echo "Start the grafana service =======>"
grafna
sleep 1
echo "Start the grafana service completed =======>"

echo "Start the example service of the gffg framework =======>"
server
sleep 1
echo "Start the example service of the gffg framework  completed=======>"

cd $(PWD)
exit 0

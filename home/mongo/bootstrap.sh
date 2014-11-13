#!/usr/bin/env bash

# Get root up in here
sudo su

# Add MongoDB to apt
apt-key adv --keyserver keyserver.ubuntu.com --recv 7F0CEB10
echo 'deb http://downloads-distro.mongodb.org/repo/ubuntu-upstart dist 10gen' | sudo tee /etc/apt/sources.list.d/10gen.list

# Update and begin installing some utility tools
apt-get -y update
apt-get install -y python-software-properties
apt-get install -y git curl
apt-get install -y build-essential

# Install latest stable version of MongoDB
apt-get install -y mongodb-10gen

MONGOD_CONF_FILE="/etc/mongod.conf"
 
tee -a $MONGOD_CONF_FILE <<-"EOF"
smallfiles = true
oplogSize = 64
replSet = rs0
EOF

service mongodb restart

echo "done!"

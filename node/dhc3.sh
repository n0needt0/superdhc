#!/usr/bin/env bash

# Get root up in here
sudo su

apt-get update

apt-get install  python-software-properties python-setuptools libtool autoconf automake uuid-dev mercurial build-essential wget git monit -y

# Add MongoDB to apt
apt-key adv --keyserver keyserver.ubuntu.com --recv 7F0CEB10
echo 'deb http://downloads-distro.mongodb.org/repo/ubuntu-upstart dist 10gen' | sudo tee /etc/apt/sources.list.d/10gen.list

# Update and begin installing some utility tools
apt-get -y update

# Install latest stable version of MongoDB
apt-get install -y mongodb-10gen

sed -i 's/# replSet = setname/replSet = rs0/g' /etc/mongodb.conf

/etc/init.d/mongodb restart

echo "installing golang"

wget https://storage.googleapis.com/golang/go1.3.3.linux-amd64.tar.gz

tar -C /usr/local -xzf go1.3.3.linux-amd64.tar.gz

rm go1.3.3.linux-amd64.tar.gz

echo 'export PATH=/vagrant/bin:/usr/local/go/bin:$PATH' >> /home/vagrant/.profile
echo 'export GOPATH=/vagrant' >> /home/vagrant/.profile

chown vagrant:vagrant /home/vagrant/.profile

wget http://download.zeromq.org/zeromq-3.2.5.tar.gz

tar -zxvf zeromq-3.2.5.tar.gz

rm zeromq-3.2.5.tar.gz

cd zeromq-3.2.5

./configure

make

sudo make install

ldconfig

cd ..

cd ..

rm -rf zeromq-3.2.5

mongo --host 192.168.42.100 << 'EOF'
config = { _id: "rs0", members:[
          { _id : 0, host : "192.168.42.100:27017"},
          { _id : 1, host : "192.168.42.110:27017"},
          { _id : 2, host : "192.168.42.120:27017"} ]
         };
rs.initiate(config);

rs.status();

EOF




/etc/init.d/mongodb restart

echo "done!"

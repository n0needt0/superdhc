#!/usr/bin/env bash

# Get root up in here
sudo su

export DEBIAN_FRONTEND=noninteractive
export NODE=node2

apt-get update

apt-get install  python-software-properties python-setuptools libtool autoconf automake uuid-dev mercurial build-essential wget curl git monit -y

# Add MongoDB to apt
apt-key adv --keyserver keyserver.ubuntu.com --recv 7F0CEB10
echo 'deb http://downloads-distro.mongodb.org/repo/ubuntu-upstart dist 10gen' | sudo tee /etc/apt/sources.list.d/10gen.list

# Update and begin installing some utility tools
apt-get -y update

# Install latest stable version of MongoDB
sudo apt-get install -y mongodb-org=2.6.1 mongodb-org-server=2.6.1 mongodb-org-shell=2.6.1 mongodb-org-mongos=2.6.1 mongodb-org-tools=2.6.1

#standar dmongo conf
cp /vagrant/etc/mongod_node.conf /etc/mongod.conf

#monit script
cp /vagrant/etc/monit/conf.d/mongo /etc/monit/conf.d/

/etc/init.d/mongod restart

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

rm -rf zeromq-3.2.5

#create log directory
mkdir -p /var/log/dhc4
chmod 777 /var/log/dhc4

#create dhc4 config dir
mkdir -p /etc/dhc4
chmod 777 /etc/dhc4

echo "$NODE" > /etc/dhc4/dhc4.cfg

#create dhc4 binary dir
mkdir -p /var/dhc4
chmod 777 /var/dhc4

#install log rotate
cp /vagrant/etc/logrotate.d/dhc4  /etc/logrotate.d/

#INSTALL CLEANER
#cleaner config
cp /vagrant/etc/dhc4/cleaner.cfg /etc/dhc4/
sed -i "s/THISNODEID/$NODE/g" /etc/dhc4/cleaner.cfg

#modify prefered server order
sed -i 's/MYTARGETS/tcp:\/\/192.168.82.110:6455,tcp:\/\/192.168.82.100:6455,tcp:\/\/192.168.82.120:6455/g' /etc/dhc4/cleaner.cfg


#cleaner binary
cp /vagrant/bin/cleaner /var/dhc4/cleaner
chmod 777 /var/dhc4/cleaner

#cleaner monit and init files
cp /vagrant/etc/init/cleaner.conf /etc/init/
cp /vagrant/etc/monit/conf.d/cleaner /etc/monit/conf.d/

/sbin/stop cleaner
/sbin/start cleaner

#INSTALL dispatch
#dispatch config
cp /vagrant/etc/dhc4/dispatch.cfg /etc/dhc4/
sed -i "s/THISNODEID/$NODE/g" /etc/dhc4/dispatch.cfg

#dispatch binary
cp /vagrant/bin/dispatch /var/dhc4/dispatch
chmod 777 /var/dhc4/dispatch

#dispatch monit and init files
cp /vagrant/etc/init/dispatch.conf /etc/init/
cp /vagrant/etc/monit/conf.d/dispatch /etc/monit/conf.d/

/sbin/stop dispatch
/sbin/start dispatch

#INSTALL NODE
#node config
cp /vagrant/etc/dhc4/node.cfg /etc/dhc4/
sed -i "s/THISNODEID/$NODE/g" /etc/dhc4/node.cfg

#node binary
cp /vagrant/bin/node /var/dhc4/node
chmod 777 /var/dhc4/node

#node monit and init files
cp /vagrant/etc/init/node.conf /etc/init/
cp /vagrant/etc/monit/conf.d/node /etc/monit/conf.d/

/sbin/stop node
/sbin/start node

#using this repo to install ganglia 3.4 as it allows for host name overwrites
add-apt-repository ppa:rufustfirefly/ganglia
# Update and begin installing some utility tools
apt-get -y update
apt-get install ganglia-monitor -y

cp /vagrant/etc/ganglia/gmond_node.conf /etc/ganglia/gmond.conf
sed -i "s/THISNODEID/$NODE/g" /etc/ganglia/gmond.conf

#install MongoDb Ganglia Support
 mkdir /usr/lib/ganglia/python_modules

 cp /vagrant/usr/lib/ganglia/python_modules/mongodb.py  /usr/lib/ganglia/python_modules/

 mkdir /etc/ganglia/conf.d
 cp /vagrant/etc/ganglia/conf.d/* /etc/ganglia/conf.d/

/etc/init.d/ganglia-monitor restart

echo "done!"

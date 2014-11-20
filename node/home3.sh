#!/usr/bin/env bash

# Get root up in here
sudo su

apt-get update

export DEBIAN_FRONTEND=noninteractive


apt-get install  python-software-properties python-setuptools libtool autoconf automake uuid-dev mercurial build-essential wget curl git monit -y

# Add MongoDB to apt
apt-key adv --keyserver keyserver.ubuntu.com --recv 7F0CEB10
echo 'deb http://downloads-distro.mongodb.org/repo/ubuntu-upstart dist 10gen' | sudo tee /etc/apt/sources.list.d/10gen.list

# Update and begin installing some utility tools
apt-get -y update

# Install latest stable version of MongoDB
sudo apt-get install -y mongodb-org=2.6.1 mongodb-org-server=2.6.1 mongodb-org-shell=2.6.1 mongodb-org-mongos=2.6.1 mongodb-org-tools=2.6.1

#standar dmongo conf
cp /vagrant/etc/mongod.conf /etc/

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

cd ..

rm -rf zeromq-3.2.5

mongo --host 192.168.82.100 << 'EOF'
config = { _id: "rs0", members:[
          { _id : 0, host : "192.168.82.100:27017"},
          { _id : 1, host : "192.168.82.110:27017"},
          { _id : 2, host : "192.168.82.120:27017"} ]
         };
rs.initiate(config);

rs.status();

EOF




/etc/init.d/mongod restart

#create log directory
mkdir -p /var/log/fortihealth
chmod 777 /var/log/fortihealth

#create fortinet config dir
mkdir -p /etc/fortihealth
chmod 777 /etc/fortihealth

#create fortihealth binary dir
mkdir -p /var/fortihealth
chmod 777 /var/fortihealth

#install log rotate
cp /vagrant/etc/logrotate.d/fortinet  /etc/logrotate.d/


#using this repo to install ganglia 3.4 as it allows for host name overwrites
add-apt-repository ppa:rufustfirefly/ganglia
# Update and begin installing some utility tools
apt-get -y update
apt-get install ganglia-monitor -y

cp /vagrant/etc/ganglia/gmond_node.conf /etc/ganglia/gmond.conf
sed -i 's/THISNODEID/home3/g' /etc/ganglia/gmond.conf

#install MongoDb Ganglia Support
 mkdir /usr/lib/ganglia/python_modules

 cp /vagrant/usr/lib/ganglia/python_modules/mongodb.py  /usr/lib/ganglia/python_modules/

 mkdir /etc/ganglia/conf.d
 cp /vagrant/etc/ganglia/conf.d/* /etc/ganglia/conf.d/

/etc/init.d/ganglia-monitor restart

echo "done!"

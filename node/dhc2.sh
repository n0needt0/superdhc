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

rm -rf zeromq-3.2.5

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

#INSTALL CLEANER
#cleaner config
cp /vagrant/etc/fortihealth/cleaner.cfg /etc/fortihealth/
sed -i 's/THISNODEID/DHC2/g' /etc/fortihealth/cleaner.cfg

#cleaner binary
cp /vagrant/bin/cleaner /var/fortihealth/cleaner
chmod 777 /var/fortihealth/cleaner

#cleaner monit and init files
cp /vagrant/etc/init/cleaner.conf /etc/init/
cp /vagrant/etc/monit/conf.d/cleaner /etc/monit/conf.d/

/sbin/stop cleaner
/sbin/start cleaner

echo "done!"

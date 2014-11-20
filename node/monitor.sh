#!/usr/bin/env bash

# Get root up in here
sudo su

export DEBIAN_FRONTEND=noninteractive

# Update and begin installing some utility tools
apt-get -y update
apt-get install -y python-software-properties
apt-get install -y git curl
apt-get install -y build-essential

#installing regular 3.1.7 repo ganglia NOT latest as it breaks shit up
apt-get update

#create stupid dirs otherwise it breaks
mkdir -p /usr/share/ganglia-webfrontend/lib/dwoo/cache
mkdir -p /usr/share/ganglia-webfrontend/lib/dwoo/compiled

chown www-data /usr/share/ganglia-webfrontend/lib/dwoo/cache
chown www-data /usr/share/ganglia-webfrontend/lib/dwoo/compiled

apt-get install rrdtool gmetad ganglia-webfrontend -y

#using this repo to install ganglia-monitor 3.4 as it allows for server name overwrites
add-apt-repository ppa:rufustfirefly/ganglia

apt-get update

apt-get install ganglia-monitor -y

cp /vagrant/etc/ganglia/gmond.conf /etc/ganglia/
sed -i 's/THISNODEID/MONITOR/g' /etc/ganglia/gmond.conf

/etc/init.d/ganglia-monitor restart

echo "done!"

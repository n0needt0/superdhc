NODE=`cat /etc/dhc4/dhc4.cfg`

stop cleaner

cp /vagrant/bin/cleaner /var/dhc4
cp /vagrant/etc/dhc4/cleaner.cfg /etc/dhc4

sed -i "s/THISNODEID/$NODE/g" /etc/dhc4/cleaner.cfg
sed -i 's/MYTARGETS/tcp:\/\/192.168.82.100:6455,tcp:\/\/192.168.82.110:6455,tcp:\/\/192.168.82.120:6455/g' /etc/dhc4/cleaner.cfg

start cleaner

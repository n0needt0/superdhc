
NODE=`cat /etc/dhc4/dhc4.cfg`

stop dispatch
stop node
stop cleaner

cp /vagrant/bin/dispatch /var/dhc4
cp /vagrant/etc/dhc4/dispatch.cfg /etc/dhc4
sed -i "s/THISNODEID/$NODE/g" /etc/dhc4/dispatch.cfg

cp /vagrant/bin/node /var/dhc4
cp /vagrant/etc/dhc4/node.cfg /etc/dhc4
sed -i "s/THISNODEID/$NODE/g" /etc/dhc4/node.cfg

cp /vagrant/bin/cleaner /var/dhc4
cp /vagrant/etc/dhc4/cleaner.cfg /etc/dhc4

sed -i "s/THISNODEID/$NODE/g" /etc/dhc4/cleaner.cfg
sed -i 's/MYTARGETS/tcp:\/\/192.168.82.100:6455,tcp:\/\/192.168.82.110:6455,tcp:\/\/192.168.82.120:6455/g' /etc/dhc4/cleaner.cfg

start node
start dispatch
start cleaner

NODE=`cat /etc/dhc4/dhc4.cfg`
stop node

cp /vagrant/bin/node /var/dhc4
cp /vagrant/etc/dhc4/node.cfg /etc/dhc4
sed -i "s/THISNODEID/$NODE/g" /etc/dhc4/node.cfg

start node

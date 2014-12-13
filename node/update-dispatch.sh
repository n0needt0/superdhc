 NODE=`cat /etc/dhc4/dhc4.cfg`

stop dispatch

cp /vagrant/bin/dispatch /var/dhc4
cp /vagrant/etc/dhc4/dispatch.cfg /etc/dhc4
sed -i "s/THISNODEID/$NODE/g" /etc/dhc4/dispatch.cfg

start dispatch

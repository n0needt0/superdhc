apt-get install libexpat1

wget http://unbound.net/downloads/unbound-1.5.1.tar.gz
tar -zxvf unbound-1.5.1.tar.gz
rm unbound-1.5.1.tar.gz

cd unbound-1.5.1

./configure --prefix=/usr

make

sudo make install

ldconfig

cd ..

rm -rf unbound-1.5.1

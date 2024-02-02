rm -f evil-tools
rm -f *.dat
go build

./evil-tools spammer --spammer.type guava --spammer.rate 20
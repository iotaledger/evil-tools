rm -f evil-tools
rm -f *.dat
go build

./evil-tools spammer --eviltools.spammer.type guava --eviltools.spammer.rate 20 --eviltools.spammer.duration 10s
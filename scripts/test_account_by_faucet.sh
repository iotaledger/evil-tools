rm -f evil-tools
rm -f *.dat
go build

./evil-tools accounts create --accounts.create.alias A

./evil-tools spammer --spammer.type blk --spammer.rate 1 --spammer.duration 10s --spammer.account A

./evil-tools accounts destroy --accounts.destroy.alias A

# this spam should not work, as account is now destroyed
./evil-tools spammer --spammer.type blk --spammer.rate 1 --spammer.duration 10s --spammer.account A

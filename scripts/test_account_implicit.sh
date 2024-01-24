rm -f evil-tools
rm -f *.dat
go build

./evil-tools accounts create --accounts.create.alias IMPLICIT --accounts.create.implicit --accounts.create.transition

./evil-tools spammer --spammer.type blk --spammer.rate 1 --spammer.duration 10s --spammer.account IMPLICIT

./evil-tools accounts destroy --accounts.destroy.alias IMPLICIT

# this spam should not work, as account is now destroyed
./evil-tools spammer --spammer.type blk --spammer.rate 1 --spammer.duration 10s --spammer.account IMPLICIT

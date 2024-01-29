# evil-tools

## Evil tools

```bash
go build
./evil-tools [spammer,accounts] [[script-flag-set]]
```

The evil-tools app has two applications: 
- spammer - to spam the network with transactions, nested conflicts, etc.
- accounts - to create, convert and destroy accounts.
List of all possible flags can be found in [configuration.md](configuration.md)

### `spammer`
Usage for spammer tool:
`./evil-tools spammer [FLAGS]`

Possible spam scenarios:
`blk, tx, peace, bb, ds, conflict-circle, guava ,orange, mango, pear, lemon, banana, kiwi`


### `accounts`
Usage for accounts tool:
`./evil-tools accounts [COMMAND] [FLAGS]`

Possible commands:
`create, convert, destroy, allot `

### Examples for the spammer
Possible
Spam with scenario `tx`
```bash
./evil-tools spammer --spammer.type tx --spammer.rate 10 --spammer.duration 100s
```
Infinite spam is enabled when no duration flag is provided.
```bash
./evil-tools spammer --spammer.type tx --spammer.rate 10
```
You can provide urls for clients and the faucet, each client should run the inx-indexer:
```bash
./evil-tools spammer --tool.nodeURLs "http://localhost:8050" --tool.faucetURL  "http://localhost:8088" --spammer.type tx --spammer.rate 10
```
Enable deep spam:
```bash
./evil-tools spammer --spammer.type tx --spammer.rate 10 --spammer.duration 100s --spammer.deepSpamEnabled
```

### Examples for the accounts
Create implicit account with alias `A`:
```bash
./evil-tools accounts create --accounts.create.alias A --accounts.create.implicit
```
Create account with genesis account paying for creation transaction:
```bash
./evil-tools accounts create --accounts.create.alias A
```
Delegate 1000 tokens (requested from the Faucet) and store it under alias `A`:
```bash
./evil-tools accounts delegate --accounts.delegate.fromAlias A --accounts.delegate.amount 100000
```
Allot at least `amount` of mana to the account with alias `A`:
```bash
./evil-tools accounts allot --accounts.allot.alias A --accounts.allot.amount 100000
``` 
Claim all rewards under alias `A`:
```bash
./evil-tools accounts claim --accounts.claim.alias A
```

### Examples for printing tool details and info about the network
List all accounts stored in the wallet.dat file of the evil-tools app:
```bash
./evil-tools info accounts
```
List all delegations done with the evil-tools app to be claimed:
```bash
./evil-tools info delegations
```
Request validators endpoint:
```bash
./evil-tools info validators
```
Request committee endpoint:
```bash
./evil-tools info committee
```
List rewards endpoint responses for all delegations done by the app:
```bash
./evil-tools info rewards
```


### Scenario diagrams:
##### No conflicts
- `single-tx`

![Single transaction](./img/evil-scenario-tx.png "Single transaction")

- `peace`

![Peace](./img/evil-scenario-peace.png "Peace")

- `bb` - blow ball structure

##### Conflicts
- `ds`

![Double spend](./img/evil-scenario-ds.png "Double spend")

- `conflict-circle`

![Conflict circle](./img/evil-scenario-conflict-circle.png "Conflict circle")

- `guava`

![Guava](./img/evil-scenario-guava.png "Guava")

- `orange`

![Orange](./img/evil-scenario-orange.png "Orange")

- `mango`

![Mango](./img/evil-scenario-mango.png "Mango")

- `pear`

![Pear](./img/evil-scenario-pear.png "Pear")

- `lemon`

![Lemon](./img/evil-scenario-lemon.png "Lemon")

- `banana`

![Banana](./img/evil-scenario-banana.png "Banana")

- `kiwi`

![Kiwi](./img/evil-scenario-kiwi.png "Kiwi")



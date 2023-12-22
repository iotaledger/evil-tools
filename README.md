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
./evil-tools spammer --eviltools.spammer.type tx --eviltools.spammer.rate 10 --eviltools.spammer.duration 100s
```
Infinite spam is enabled when no duration flag is provided.
```bash
./evil-tools spammer --eviltools.spammer.type tx --eviltools.spammer.rate 10
```
You can provide urls for clients:
```bash
./evil-tools spammer --eviltools.spammer.urls "http://localhost:8050,http://localhost:8060" --eviltools.spammer.type tx --eviltools.spammer.rate 10
```
Enable deep spam:
```bash
./evil-tools spammer --eviltools.spammer.type tx --eviltools.spammer.rate 10 --eviltools.spammer.duration 100s --eviltools.spammer.deep
```

### Examples for the accounts
Create implicit account with alias `A`:
```bash
./evil-tools accounts create --eviltools.accounts.create.alias A --eviltools.accounts.create.implicit
```
Create account with genesis account paying for creation transaction:
```bash
./evil-tools accounts create --eviltools.accounts.create.alias A
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



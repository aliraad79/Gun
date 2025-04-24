# Gun
modern high performance match engine


## Design Decisions
### Should process 500k orders per second
This means it should publish matches data in at most one second if the input order rate is about 500k messages


### No nned to restart
This module is not needed to be shut down and then power up everytime you need to add a orderbook and stop an existing one


## load test
```bash
cd loadTest
go run .
```
## topics
creating an Order -> NewOrder


## complexity
create a new order -> O(n) that n is number of price rows in order side
cancel a order -> O(nm) that n is number of price rows in order side and m orders in that price

both can be replace by log(n) using binary search


## storage
For now this is memory match engine that can reaplay on the kafka messages and create last state of match engine up untill now


## Assumtations
Must add market order

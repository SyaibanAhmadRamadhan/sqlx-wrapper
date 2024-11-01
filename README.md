# WSQLX
WSQLX is a library that serves as a wrapper for the [sqlx](https://github.com/jmoiron/sqlx) library. It integrates with OpenTelemetry for logging and uses [Squirrel](https://github.com/Masterminds/squirrel) as a query builder. The library also includes a database transaction closure feature, which you can use in the service layer to support ACID properties across different repository layers. For transaction mocking needs, WSQLX leverages [Uber Go Mock](https://github.com/uber-go/mock). Additionally, it includes custom query support for pagination.

## Tag Versioning Example: `v1.231215.2307`
We use a time-based versioning (TBD) scheme for our releases. The format is as follows:
```txt
v1.yearMonthDate.HourMinute
```
- `year`: Last two digits of the current year (e.g., 23 for 2023).
- `month`: Two-digit month (e.g., 12 for December).
- `date`: Two-digit day of the month (e.g., 15).
- `HourMinute`: Time of release in 24-hour format, combined as HHMM (e.g., 2307 for 11:07 PM).

## Initial rdbms 
install sqlx wrapper
```shell
go get github.com/SyaibanAhmadRamadhan/sqlx-wrapper@v1.241102.0023
```

```Go
// Initialize the Rdbms wrapper with an existing sqlx.DB instance.
sqlxWrapper := wsqlx.NewRdbms(&sqlx.DB{})

// Pass the wrapped Rdbms instance to the repository.
repository := NewRepository(sqlxWrapper)
```

## how to use rdbms
You can use the `Rdbms` interface for queries in the repository layer. Here's an example:
```Go
type repository struct {
    sqlx db.Rdbms
    sq   squirrel.StatementBuilderType
}

// NewRepository creates a new repository with the provided Rdbms implementation.
func NewRepository(sqlx db.Rdbms) *repository {
    return &repository{
        sqlx: sqlx,
        sq:   squirrel.StatementBuilder.PlaceholderFormat(squirrel.Question),
    }
}

// GetAll retrieves all bank accounts with optional filtering and pagination.
func (r *repository) GetAll(ctx context.Context, input GetAllInput) (output GetAllOutput, err error) {
    // Build the main query for selecting bank accounts.
    query := r.sq.Select(
    "id", "consumer_id", "name", "account_number", "account_holder_name",
    ).From("bank_accounts")

    // Build the query for counting the total number of bank accounts.
    queryCount := r.sq.Select("COUNT(*)").From("bank_accounts")

    // Apply filtering based on the ConsumerID if provided.
    if input.ConsumerID.Valid {
        query = query.Where(squirrel.Eq{"consumer_id": input.ConsumerID.Int64})
        queryCount = queryCount.Where(squirrel.Eq{"consumer_id": input.ConsumerID.Int64})
    }

    // Initialize the output with an empty list of items.
    output = GetAllOutput{
        Items: make([]GetAllOutputItem, 0),
    }

    // Execute the paginated query and map the results to the output items.
    output.Pagination, err = r.sqlx.QuerySqPagination(ctx, queryCount, query, input.Pagination, func(rows *sqlx.Rows) (err error) {
        for rows.Next() {
            item := GetAllOutputItem{}
            if err = rows.StructScan(&item); err != nil {
                return tracer.Error(err)
            }
            output.Items = append(output.Items, item)
        }
        return nil
    })
    if err != nil {
        return output, tracer.Error(err)
    }

    return output, nil
}
```
Explanation:
- **Repository Structure**: The repository struct holds the Rdbms interface for database interactions and the Squirrel statement builder for constructing SQL queries.
- **NewRepository Function**: This function initializes a new repository instance, setting up the SQL builder with `squirrel.Question` for placeholder formatting.

## How to Use Transaction DB Tx
You can use the `Rdbms` interface for queries in the service layer. Below is an example implementation.

in `service.go`
```Go
type service struct {
    bankAccountRepository bank_accounts.Repository
    dbTx                  db.Tx
}

var _ Service = (*service)(nil)

type NewServiceOpts struct {
    BankAccountRepository bank_accounts.Repository
    DBTx                  db.Tx
}

func NewService(opts NewServiceOpts) *service {
    return &service{
        bankAccountRepository: opts.BankAccountRepository,
        dbTx:                  opts.DBTx,
    }
}

func (s *service) Creates(ctx context.Context, input CreatesInput) (err error) {
    const batch = 10
    
    // Split the input items into batches for processing.
    itemBatches := util.SplitDataIntoBatch(input.Items, batch)
    if len(itemBatches) <= 0 {
        return
    }

    // Execute the transaction with the specified options.
    err = s.dbTx.DoTransaction(ctx, &sql.TxOptions{
    Isolation: sql.LevelReadCommitted,
    ReadOnly:  false,
    }, func(tx db.Rdbms) error {
            // Process each batch of items within the transaction.
            for i, items := range itemBatches {
            createsBankAccountItems := make([]bank_accounts.CreatesInputItem, 0)
            
            for _, item := range items {
                createsBankAccountItems = append(createsBankAccountItems, bank_accounts.CreatesInputItem{
                    ConsumerID:        input.ConsumerID,
                    Name:              item.Name,
                    AccountNumber:     item.AccountNumber,
                    AccountHolderName: item.AccountHolderName,
                })
            }
            
            // Call the repository method to create the bank account items.
            err = s.bankAccountRepository.Creates(ctx, bank_accounts.CreatesInput{
                Transaction: tx,
                Items:       createsBankAccountItems,
            })
            
            // Handle errors in the batch processing.
            if err != nil {
                return tracer.Error(fmt.Errorf("error in batch %d: %w", i, err))
            }
        }
        return nil
    })
    if err != nil {
        return tracer.Error(err)
    }
    
    return nil
}
```

in repo layer
```Go
type repository struct {
	sqlx db.Rdbms
	sq   squirrel.StatementBuilderType
}

func NewRepository(sqlx db.Rdbms) *repository {
	return &repository{
		sqlx: sqlx,
		sq:   squirrel.StatementBuilder.PlaceholderFormat(squirrel.Question),
	}
}

type CreatesInput struct {
    Transaction db.Rdbms
    Items       []CreatesInputItem
}

type CreatesInputItem struct {
    ConsumerID        int64
    Name              string
    AccountNumber     string
    AccountHolderName string
}

func (r *repository) Creates(ctx context.Context, input CreatesInput) (err error) {
    if input.Items == nil || len(input.Items) == 0 {
        return
    }

    rdbms := r.sqlx
    if input.Transaction != nil {
        rdbms = input.Transaction
    }

    query := r.sq.Insert("bank_accounts").Columns(
    "consumer_id", "name", "account_number", "account_holder_name",
    )

    for _, item := range input.Items {
        query = query.Values(item.ConsumerID, item.Name, item.AccountNumber, item.AccountHolderName)
    }

    _, err = rdbms.ExecSq(ctx, query)
    if err != nil {
        return tracer.Error(err)
    }
	
    return
}
```

## Contact
For questions or support, please contact ibanrama29@gmail.com.
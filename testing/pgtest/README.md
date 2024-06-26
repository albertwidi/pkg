# Testing Package For Postgres

The testing package for PostgreSQL provodides helper functions and `clone_schema` for testing purposes.

> [!WARNING]
> Please use this package carefully as this package provide functions to CREATE and DROP a database.

## Helper Functions

1. Create Database.

   User is allowed to create a new database via `CreateDatabase` function. The function will `forcefully` creates the database, which means it will be `drop` the existing database and create a new one if needed.

1. Drop Database.

   User is allowed to drop a database is it no-longer used.

1. Apply Schema.

1. ForkConnWithNewSchema.

## Parallel Testing With Multiple Schemas

The package allows the user to perform parallel testing to a single Postgres database by separating [Postgresql Schema](https://www.postgresql.org/docs/current/ddl-schemas.html) for each connection/session. Currently, it only supports the `public` schema.

**Limitation**

Currently, the test helper only taking `public` schema as the source of truth. And it expects everything is created within the `public` schema, so it won't work for multi-schema setup.

If you create everything in the `public` schema then the helper functions will work as expected.

**One Session One Schema**

When doing integration tests for a program, the program/test need to be run sequentially. If not, then there will a high chance of data-race and failing tests all over the place because they are relying on one data source and schema. This is why,
it's very important to make the data to be separated per test cases to recuce the chance of data race and flaky tests.

But, separating the data is sometimes not enough, because multiple tests cases might need to see the data in the same table but with different condition or flag. This bring us back to the first problem, tests need to be run in sequential order.

While running tests in sequential order solves the problem and good enough for small to medium sized codebase, this can be very slow for a large codebase where we have thousands of integration tests touching the database. To speed-up this process,
the tests need to be running in parallel and in a different schema for different tests cases.

```mermaid
flowchart LR
	Test.Main --> Test1 --> Schema.Test1
	Test.Main --> Test2 --> Schema.Test2
	Test.Main --> Test3 --> Schema.Test3
	Test.Main --> ... --> Schema....["Schema.{...}"]
	Test.Main --> N --> Schema.N
```

**How Schema Cloning Maintained?**

When doing migration, we will always keeping the `baseline` called `baseline.schema`. This will ensure the whole structure will never changed and we don't need to wait for any locks(other than schema clone) to clone the whole schema to another schema.

```mermaid
flowchart LR
	TM[Test.Main] --Run 1st--> MU[M1igrate Up] --> BS[Baseline.Schema]
	TM --> T1[Test1] --Connect--> T1S[Test1.Session] --Apply--> T1SC[Test1.Schema] --> RunTest
	T1SC --Clone --> BS
	TM --> T2[Test2] --Connect--> T2S[Test2.Session] --Apply--> T2SC[Test2.Schema] --> RunTest
	T2SC --Clone --> BS
	TM --> T3[Test3] --Connect--> T3S[Test3.Session] --Apply--> T3SC[Test3.Schema] --> RunTest
	T3SC --Clone --> BS
```

**Clone Schema Function**

The `clone_schema.sql` is embedded in the library and used to create `clone` function for each database created using the library.

The `clone_schema.sql` is **only** applied when a new databse is created and if only it haven't been created before.

```mermaid
flowchart LR
  TM[Test.Main] --Create--> TH[Test Helper]
```
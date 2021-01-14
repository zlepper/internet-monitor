create table results(
    id serial primary key,
    time TIMESTAMP not null,
    duration interval,
    url text not null,
    had_error bool not null
)
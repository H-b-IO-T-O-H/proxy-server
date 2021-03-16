drop table if exists requests;

create unlogged table requests (
    req_id serial constraint req_id_pkey primary key,
    uri text,
    IsHttps bool,
    data bytea
);
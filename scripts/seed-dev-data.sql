-- scripts/seed-dev-data.sql
INSERT INTO probe.sites (name, external_ip, port, isp_name, isp_asn) VALUES
    ('Obiekt-Warsaw-Orange',   '1.2.3.4',   554, 'Orange',   5617),
    ('Obiekt-Krakow-TMobile',  '5.6.7.8',   443, 'T-Mobile', 29535),
    ('Obiekt-Gdansk-Orange',   '9.10.11.12', 80, 'Orange',   5617),
    ('Obiekt-Poznan-Play',     '13.14.15.16', 8080, 'Play',  39603)
ON CONFLICT DO NOTHING;
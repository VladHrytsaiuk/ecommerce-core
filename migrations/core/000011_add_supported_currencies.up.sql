-- Keep PostgreSQL and the Money value object on the same ISO-4217 registry.
-- The seed is the versioned registry provided by golang.org/x/text/currency
-- (including historical and non-tender ISO units, excluding XXX). Update this
-- migration set only through a new forward migration when that dependency is
-- upgraded and its registry changes.
CREATE TABLE supported_currencies (
    code CHAR(3) PRIMARY KEY,
    CHECK (code ~ '^[A-Z]{3}$')
);

INSERT INTO supported_currencies (code) VALUES
('ADP'),('AED'),('AFA'),('AFN'),('ALK'),('ALL'),('AMD'),('ANG'),('AOA'),('AOK'),('AON'),('AOR'),('ARA'),('ARL'),('ARM'),('ARP'),('ARS'),('ATS'),('AUD'),('AWG'),('AZM'),('AZN'),
('BAD'),('BAM'),('BAN'),('BBD'),('BDT'),('BEC'),('BEF'),('BEL'),('BGL'),('BGM'),('BGN'),('BGO'),('BHD'),('BIF'),('BMD'),('BND'),('BOB'),('BOL'),('BOP'),('BOV'),('BRB'),('BRC'),('BRE'),('BRL'),('BRN'),('BRR'),('BRZ'),('BSD'),('BTN'),('BUK'),('BWP'),('BYB'),('BYN'),('BYR'),('BZD'),
('CAD'),('CDF'),('CHE'),('CHF'),('CHW'),('CLE'),('CLF'),('CLP'),('CNH'),('CNX'),('CNY'),('COP'),('COU'),('CRC'),('CSD'),('CSK'),('CUC'),('CUP'),('CVE'),('CYP'),('CZK'),
('DDM'),('DEM'),('DJF'),('DKK'),('DOP'),('DZD'),
('ECS'),('ECV'),('EEK'),('EGP'),('ERN'),('ESA'),('ESB'),('ESP'),('ETB'),('EUR'),
('FIM'),('FJD'),('FKP'),('FRF'),
('GBP'),('GEK'),('GEL'),('GHC'),('GHS'),('GIP'),('GMD'),('GNF'),('GNS'),('GQE'),('GRD'),('GTQ'),('GWE'),('GWP'),('GYD'),
('HKD'),('HNL'),('HRD'),('HRK'),('HTG'),('HUF'),
('IDR'),('IEP'),('ILP'),('ILR'),('ILS'),('INR'),('IQD'),('IRR'),('ISJ'),('ISK'),('ITL'),
('JMD'),('JOD'),('JPY'),
('KES'),('KGS'),('KHR'),('KMF'),('KPW'),('KRH'),('KRO'),('KRW'),('KWD'),('KYD'),('KZT'),
('LAK'),('LBP'),('LKR'),('LRD'),('LSL'),('LTL'),('LTT'),('LUC'),('LUF'),('LUL'),('LVL'),('LVR'),('LYD'),
('MAD'),('MAF'),('MCF'),('MDC'),('MDL'),('MGA'),('MGF'),('MKD'),('MKN'),('MLF'),('MMK'),('MNT'),('MOP'),('MRO'),('MTL'),('MTP'),('MUR'),('MVR'),('MWK'),('MXN'),('MXP'),('MXV'),('MYR'),('MZE'),('MZM'),('MZN'),
('NAD'),('NGN'),('NIC'),('NIO'),('NLG'),('NOK'),('NPR'),('NZD'),
('OMR'),
('PAB'),('PEI'),('PEN'),('PES'),('PGK'),('PHP'),('PKR'),('PLN'),('PLZ'),('PTE'),('PYG'),
('QAR'),
('RHD'),('ROL'),('RON'),('RSD'),('RUB'),('RUR'),('RWF'),
('SAR'),('SBD'),('SCR'),('SDD'),('SDG'),('SDP'),('SEK'),('SGD'),('SHP'),('SIT'),('SKK'),('SLL'),('SOS'),('SRD'),('SRG'),('SSP'),('STD'),('STN'),('SUR'),('SVC'),('SYP'),('SZL'),
('THB'),('TJR'),('TJS'),('TMM'),('TMT'),('TND'),('TOP'),('TPE'),('TRL'),('TRY'),('TTD'),('TWD'),('TZS'),
('UAH'),('UAK'),('UGS'),('UGX'),('USD'),('USN'),('USS'),('UYI'),('UYP'),('UYU'),('UZS'),
('VEB'),('VEF'),('VND'),('VNN'),('VUV'),
('WST'),
('XAF'),('XAG'),('XAU'),('XBA'),('XBB'),('XBC'),('XBD'),('XCD'),('XDR'),('XEU'),('XFO'),('XFU'),('XOF'),('XPD'),('XPF'),('XPT'),('XRE'),('XSU'),('XTS'),('XUA'),
('YDD'),('YER'),('YUD'),('YUM'),('YUN'),('YUR'),
('ZAL'),('ZAR'),('ZMK'),('ZMW'),('ZRN'),('ZRZ'),('ZWD'),('ZWL'),('ZWR');

ALTER TABLE product_variants ADD CONSTRAINT product_variants_currency_supported_fkey
    FOREIGN KEY (currency) REFERENCES supported_currencies(code) NOT VALID;
ALTER TABLE orders ADD CONSTRAINT orders_currency_supported_fkey
    FOREIGN KEY (currency) REFERENCES supported_currencies(code) NOT VALID;
ALTER TABLE order_items ADD CONSTRAINT order_items_currency_supported_fkey
    FOREIGN KEY (currency) REFERENCES supported_currencies(code) NOT VALID;
ALTER TABLE payments ADD CONSTRAINT payments_currency_supported_fkey
    FOREIGN KEY (currency) REFERENCES supported_currencies(code) NOT VALID;
ALTER TABLE payment_checkout_attempts ADD CONSTRAINT payment_checkout_attempts_currency_supported_fkey
    FOREIGN KEY (currency) REFERENCES supported_currencies(code) NOT VALID;
ALTER TABLE payment_webhook_events ADD CONSTRAINT payment_webhook_events_currency_supported_fkey
    FOREIGN KEY (currency) REFERENCES supported_currencies(code) NOT VALID;
ALTER TABLE payment_anomalies ADD CONSTRAINT payment_anomalies_currency_supported_fkey
    FOREIGN KEY (currency) REFERENCES supported_currencies(code) NOT VALID;

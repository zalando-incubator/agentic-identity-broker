ALTER TABLE thirdparty_oauth2_services
    ADD COLUMN icon_svg TEXT,
    ADD CONSTRAINT chk_thirdparty_oauth2_services_icon_svg
        CHECK (icon_svg IS NULL OR octet_length(icon_svg) BETWEEN 1 AND 65536);

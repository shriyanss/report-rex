"""Tests for LaTeX chart generation, including the asset (rex ingest) charts."""

from report_rex_core import charts


class TestRenderAssets:
    def test_pie_by_os(self):
        assets = [{"os": "Linux"}, {"os": "Linux"}, {"os": "Windows"}, {}]
        result = charts.render_assets(assets, "pie", "os")
        assert "tikzpicture" in result
        assert "Linux" in result
        assert "Unknown" in result  # missing os falls back to "Unknown"

    def test_bar_by_ports(self):
        assets = [
            {"ports": [{"number": "22", "proto": "tcp"}, {"number": "80", "proto": "tcp"}]},
            {"ports": [{"number": "22", "proto": "tcp"}]},
        ]
        result = charts.render_assets(assets, "bar", "ports")
        assert r"22/\allowbreak{}tcp" in result
        assert r"80/\allowbreak{}tcp" in result

    def test_services_dataset(self):
        assets = [{"ports": [{"number": "22", "service": "ssh"}, {"number": "23", "service": "ssh"}]}]
        result = charts.render_assets(assets, "bar", "services")
        assert "ssh" in result

    def test_single_slice_pie_draws_full_disc(self):
        result = charts.render_assets([{}], "pie", "os")
        assert "hide number" in result
        assert "circle (2.3)" in result
        assert "at (0,0) {1}" in result

    def test_multi_slice_pie_keeps_pgf_pie_labels(self):
        result = charts.render_assets([{"os": "Linux"}, {"os": "Windows"}], "pie", "os")
        assert "hide number" not in result

    def test_donut_labels_sit_in_ring_and_total_is_captioned(self):
        # #79: High 1, Medium 1, Low 2 — pgf-pie's labels landed under the hole.
        findings = [{"severity": s} for s in ("High", "Medium", "Low", "Low")]
        result = charts.render(findings, "donut", "severity")
        assert "hide number" in result
        for angle, v in (("45.0", 1), ("135.0", 1), ("270.0", 2)):
            assert f"at ({angle}:1.725) {{{v}}}" in result
        assert r"{\bfseries\large 4}\\{\scriptsize Total}" in result

    def test_empty_assets_renders_placeholder(self):
        result = charts.render_assets([], "pie", "os")
        assert "No data available" in result

    def test_top_n_caps_bar_entries(self):
        assets = [{"ports": [{"number": str(p), "proto": "tcp"}]} for p in range(1, 30)]
        result = charts.render_assets(assets, "bar", "ports")
        assert result.count("filldraw") <= charts._TOP_N_ASSET_ENTRIES

    def test_table_type_renders_full_inventory(self):
        assets = [
            {"ip": "10.0.0.2", "hostnames": ["b.example.com"], "os": "Linux",
             "ports": [{"number": "80", "proto": "tcp", "service": "http"}]},
            {"ip": "10.0.0.1", "hostnames": ["a.example.com"],
             "ports": [{"number": "22", "proto": "tcp", "service": "ssh"}]},
        ]
        result = charts.render_assets(assets, "table", "ports")
        assert "tblr" in result
        assert "Unknown" in result  # 10.0.0.1 has no os
        assert result.index(r"10.\allowbreak{}0.\allowbreak{}0.\allowbreak{}1") < result.index(
            r"10.\allowbreak{}0.\allowbreak{}0.\allowbreak{}2"
        )  # sorted by IP

    def test_table_type_hostname_is_breakable(self):
        assets = [{"ip": "10.0.0.1", "hostnames": ["a.example.com"]}]
        result = charts.render_assets(assets, "table", "ports")
        assert r"a.\allowbreak{}example.\allowbreak{}com" in result

    def test_table_type_host_with_no_open_ports_still_gets_a_row(self):
        result = charts.render_assets([{"ip": "10.0.0.1"}], "table", "ports")
        assert r"10.\allowbreak{}0.\allowbreak{}0.\allowbreak{}1" in result

    def test_table_type_empty_assets_renders_placeholder(self):
        result = charts.render_assets([], "table", "ports")
        assert "No data available" in result


class TestRenderWeb:
    def test_bar_by_host(self):
        findings = [{"host": "a.com"}, {"host": "a.com"}, {"host": "b.com"}]
        result = charts.render_web(findings, "bar", "host")
        assert r"a.\allowbreak{}com" in result
        assert r"b.\allowbreak{}com" in result

    def test_missing_host_falls_back_to_unknown(self):
        result = charts.render_web([{}], "pie", "host")
        assert "Unknown" in result

    def test_empty_findings_renders_placeholder(self):
        result = charts.render_web([], "bar", "host")
        assert "No data available" in result

    def test_unknown_chart_type(self):
        result = charts.render_web([{"host": "a.com"}], "matrix", "host")
        assert "Unknown chart type." in result


class TestRenderNetwork:
    ASSETS = [
        {"ip": "10.0.0.5", "os": "Microsoft Windows Server 2019", "hostnames": ["dc01.corp.local"]},
        {"ip": "10.0.0.9", "os": "Linux 5.4"},
        {"ip": "10.0.1.3"},
    ]

    def test_groups_hosts_into_auto_24s_with_os_icons(self):
        out = charts.render_network(self.ASSETS, {})
        assert out.count(r"\faNetworkWired") == 2  # one switch per subnet: 10.0.0.0/24 and 10.0.1.0/24
        assert r"\faWindows" in out and r"\faLinux" in out and r"\faServer" in out
        assert r"dc01.\allowbreak{}corp" in out

    def test_labelled_subnet_wins_over_auto_24(self):
        out = charts.render_network(self.ASSETS, {"subnets": {"10.0.0.0/16": "Corporate LAN"}})
        assert out.count(r"\faNetworkWired") == 1
        assert "Corporate LAN" in out

    def test_pivots_draw_arrows_and_add_unknown_endpoints(self):
        conn = {"pivots": {"10.0.0.5": ["172.16.5.0/24", "192.168.1.7"], "10.0.1.0/24": "10.0.0.9"}}
        out = charts.render_network(self.ASSETS, conn)
        assert out.count("to[bend left=12]") == 3
        assert r"172.\allowbreak{}16" in out  # new empty subnet column
        assert r"192.\allowbreak{}168.\allowbreak{}1.\allowbreak{}7" in out  # new host

    def test_wide_diagram_is_scaled_down(self):
        assets = [{"ip": f"10.0.{i}.1"} for i in range(8)]
        assert "scale=1.000" not in charts.render_network(assets, {})

    def test_empty_renders_placeholder(self):
        assert "No data available" in charts.render_network([], {})

    def test_crowded_subnet_hides_unrelated_hosts_first(self):
        assets = [{"ip": f"10.0.0.{i}"} for i in range(1, 26)]
        conn = {"pivots": {"10.0.0.25": "10.0.0.24"}, "important": ["10.0.0.23"]}
        out = charts.render_network(assets, conn)
        for kept in ("25", "24", "23", "1", "9"):  # pivots, important, then IP order
            assert rf"0.\allowbreak{{}}{kept}}}" in out
        assert r"0.\allowbreak{}10}" not in out
        assert "+ 13 more hosts" in out
        assert "nd, imp" in out  # important host outlined

    def test_legend_lists_only_present_families(self):
        out = charts.render_network(self.ASSETS, {})
        assert "Windows" in out and "Linux" in out and "macOS" not in out
        assert "Pivot}" not in out and "Important host" not in out

    def test_must_show_hosts_overflow_scales_down(self):
        assets = [{"ip": f"10.0.0.{i}"} for i in range(1, 31)]
        conn = {"important": [f"10.0.0.{i}" for i in range(1, 31)]}
        out = charts.render_network(assets, conn)
        assert "more hosts" not in out
        assert "scale=1.000" not in out


FINDINGS = [
    {"severity": "Critical", "business_risk": ["Operational"], "material_impact": "Proprietary Data Loss"},
    {"severity": "High", "business_risk": "Financial, Operational", "material_impact": "Proprietary Data Loss"},
    {"severity": "Low", "material_impact": "Information Privacy"},
]


class TestRadarAndMaterial:
    def test_business_radar_has_a_spoke_per_category(self):
        result = charts.render(FINDINGS, "radar", "business")
        assert result.count("(0,0) --") == 5  # all five categories, zeros included
        assert r"Strategic \textbf{(0)}" in result
        assert r"Operational \textbf{(2)}" in result

    def test_radar_falls_back_to_bar_below_three_spokes(self):
        result = charts.render(FINDINGS, "spider", "material")
        assert "rounded corners" in result

    def test_material_matrix(self):
        result = charts.render(FINDINGS, "matrix", "fair-mam")
        assert "Proprietary Data Loss" in result
        assert "Information Privacy" in result

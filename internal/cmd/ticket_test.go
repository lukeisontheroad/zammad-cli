package cmd

import "testing"

func TestCustomerQuery(t *testing.T) {
	cases := map[string]string{
		"ACME":             `(customer.email:*acme* OR organization.name:"ACME" OR "ACME")`,
		"acme.com":         `(customer.email:*acme.com* OR organization.name:"acme.com" OR "acme.com")`,
		"jane@example.com": `(customer.email:"jane@example.com" OR organization.name:"jane@example.com" OR "jane@example.com")`,
		// Multi-word terms cannot be an email wildcard.
		"ACME Corporation Ltd": `(organization.name:"ACME Corporation Ltd" OR "ACME Corporation Ltd")`,
	}
	for in, want := range cases {
		if got := customerQuery(in); got != want {
			t.Errorf("customerQuery(%q)\n got:  %s\n want: %s", in, got, want)
		}
	}
}

func TestParseTicketID(t *testing.T) {
	good := map[string]int{
		"58013":  58013,
		"#58013": 58013,
		"https://contact.example.com/#ticket/zoom/58013":     58013,
		"https://contact.example.com/#ticket/zoom/58013/foo": 58013,
	}
	for in, want := range good {
		id, err := parseTicketID(in)
		if err != nil || id != want {
			t.Errorf("parseTicketID(%q) = %d, %v; want %d", in, id, err, want)
		}
	}
	for _, in := range []string{"", "abc", "-3", "https://x/#ticket/zoom/abc"} {
		if _, err := parseTicketID(in); err == nil {
			t.Errorf("parseTicketID(%q) should fail", in)
		}
	}
}

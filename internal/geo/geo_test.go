package geo

import "testing"

func TestHaversine(t *testing.T) {
	// Banja Luka → Tivat ≈ 330 km
	d := Haversine(44.7722, 17.1910, 42.4347, 18.7060)
	if d < 280000 || d > 295000 {
		t.Fatalf("distance = %.0f m", d)
	}
}

func TestSimplify(t *testing.T) {
	var pts []Pt
	for i := 0; i < 100; i++ {
		pts = append(pts, Pt{Lat: 44.0 + float64(i)*0.0001, Lng: 17.0})
	}
	out := Simplify(pts, 5)
	if len(out) != 2 {
		t.Fatalf("straight line should collapse to 2 points, got %d", len(out))
	}
	pts[50].Lng = 17.01
	out = Simplify(pts, 5)
	if len(out) < 3 || len(out) > 6 {
		t.Fatalf("one spike should keep a few points, got %d", len(out))
	}
}

func TestWeatherLabel(t *testing.T) {
	if WeatherLabel(0) != "Clear" || WeatherLabel(61) != "Rain" || WeatherLabel(95) != "Thunderstorm" {
		t.Fatal("labels")
	}
}

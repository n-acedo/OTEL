package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

type Cep struct {
	Cep         string `json:"cep"`
	Logradouro  string `json:"logradouro"`
	Complemento string `json:"complemento"`
	Bairro      string `json:"bairro"`
	Localidade  string `json:"localidade"`
	Uf          string `json:"uf"`
	Unidade     string `json:"unidade"`
	Ibge        string `json:"ibge"`
	Gia         string `json:"gia"`
	Erro        string `json:"erro"`
}

type Current struct {
	LastUpdatedEpoch int     `json:"last_updated_epoch"`
	LastUpdated      string  `json:"last_updated"`
	TempC            float64 `json:"temp_c"`
	TempF            float64 `json:"temp_f"`
	IsDay            int     `json:"is_day"`
	Condition        struct {
		Text string `json:"text"`
		Icon string `json:"icon"`
		Code int    `json:"code"`
	} `json:"condition"`
	WindMph    float64 `json:"wind_mph"`
	WindKph    float64 `json:"wind_kph"`
	WindDegree float64 `json:"wind_degree"`
	WindDir    string  `json:"wind_dir"`
	PressureMb float64 `json:"pressure_mb"`
	PressureIn float64 `json:"pressure_in"`
	PrecipMm   float64 `json:"precip_mm"`
	PrecipIn   float64 `json:"precip_in"`
	Humidity   float64 `json:"humidity"`
	Cloud      float64 `json:"cloud"`
	FeelslikeC float64 `json:"feelslike_c"`
	FeelslikeF float64 `json:"feelslike_f"`
	WindchillC float64 `json:"windchill_c"`
	WindchillF float64 `json:"windchill_f"`
	HeatindexC float64 `json:"heatindex_c"`
	HeatindexF float64 `json:"heatindex_f"`
	DewpointC  float64 `json:"dewpoint_c"`
	DewpointF  float64 `json:"dewpoint_f"`
	VisKm      float64 `json:"vis_km"`
	VisMiles   float64 `json:"vis_miles"`
	Uv         float64 `json:"uv"`
	GustMph    float64 `json:"gust_mph"`
	GustKph    float64 `json:"gust_kph"`
}

type Weather struct {
	Location struct {
		Name           string  `json:"name"`
		Region         string  `json:"region"`
		Country        string  `json:"country"`
		Lat            float64 `json:"lat"`
		Lon            float64 `json:"lon"`
		TzID           string  `json:"tz_id"`
		LocaltimeEpoch int     `json:"localtime_epoch"`
		Localtime      string  `json:"localtime"`
	} `json:"location"`
	Current Current `json:"current"`
}

type Response struct {
	Celsius    float64 `json:"temp_C"`
	Fahrenheit float64 `json:"temp_F"`
	Kelvin     float64 `json:"temp_K"`
}

func main() {
	Tracing()

	http.HandleFunc("/", Handler)
	http.ListenAndServe(":8083", nil)
}

func Tracing() {
	exporter, err := zipkin.New("http://zipkin:9411/api/v2/spans")
	if err != nil {
		panic("Error to create exporter")
	}

	traceProvider := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("service-b"),
		)),
	)

	otel.SetTracerProvider(traceProvider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
}

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	cep := r.URL.Query().Get("cep")
	ctx, span := otel.Tracer("service-b").Start(r.Context(), "start")
	defer span.End()

	if cep == "" || len(cep) != 8 {
		http.Error(w, "invalid zipcode", http.StatusUnprocessableEntity)
		return
	}

	address, err := GetLocation(ctx, cep)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if address.Erro == "true" {
		http.Error(w, "cannot find zipcode", http.StatusNotFound)
		return
	}

	city := address.Localidade
	weather, err := GetWeather(ctx, city)

	if err != nil {
		http.Error(w, "error to get weather", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := Response{
		Celsius:    weather.TempC,
		Fahrenheit: weather.TempF,
		Kelvin:     weather.TempC + 273,
	}

	json.NewEncoder(w).Encode(response)
}

func GetLocation(ctx context.Context, cep string) (*Cep, error) {
	_, span := otel.Tracer("service-b").Start(ctx, "getting location")
	defer span.End()

	resp, err := http.Get("http://viacep.com.br/ws/" + cep + "/json/")

	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return nil, err
	}

	var address Cep

	err = json.Unmarshal(body, &address)

	if err != nil {
		return nil, err
	}

	return &address, nil
}

func GetWeather(ctx context.Context, city string) (*Current, error) {
	_, span := otel.Tracer("service-b").Start(ctx, "getting weather")
	defer span.End()

	formatCity := url.QueryEscape(city)
	resp, err := http.Get(fmt.Sprintf("http://api.weatherapi.com/v1/current.json?q=%s&key=f32a666acf614d978db184424251103", formatCity))

	if resp.StatusCode != 200 {
		return nil, errors.New("error to get weather")
	}

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return nil, err
	}

	var weather Weather

	err = json.Unmarshal(body, &weather)
	if err != nil {
		return nil, err
	}

	current := weather.Current

	return &current, nil
}

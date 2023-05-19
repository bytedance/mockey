
<a name="readme-top"> </a>

[![Contributors][contributors-shield]][contributors-url]

<!-- PROJECT LOGO -->
<br />
<div align="center">
  <h3 align="center">Mockey</h3>

  <p align="center">
  TODO: brief in one line.
    <br />
    <a href="">(TODO):DemoLink</a>
    ·
    <a href="https://github.com/bytedance/mockey/issues">Report Bug</a>
    ·
    <a href="https://github.com/bytedance/mockey/issues">:Request Feature</a>
  </p>
</div>

<!-- TABLE OF CONTENTS -->
<details>
  <summary>Table of Contents</summary>
  <ol>
    <li>
      <a href="#about-mockey">About Mockey</a>
      <ul>
        <li><a href="#built-with">Built With</a></li>
      </ul>
    </li>
    <li>
      <a href="#quick-start">Quick Start</a>
      <ul>
        <li><a href="#installation">Installation</a></li>
        <li><a href="#demo">Demo</a></li>
      </ul>
    </li>
    <li><a href="#examples">Examples</a>
	 <ul>
    <li><a href="#mock-func">Mock function</a>
    <li><a href="#mock-struct">Mock struct function</a>
    <li><a href="#mock-value">Mock value</a>
      </ul>
	</li>
    <li><a href="#usage">Usage</a></li>
    <li><a href="#roadmap">Roadmap</a></li>
    <li><a href="#contributing">Contributing</a></li>
    <li><a href="#license">License</a></li>
    <!-- <li><a href="#contact">Contact</a></li> -->
    <!-- <li><a href="#acknowledgments">Acknowledgments</a></li> -->
  </ol>
</details>



<a name="about-mockey"> </a>

<!-- ABOUT THE PROJECT -->
## About Mockey

Mockey is a simple and easy-to-use golang testing framework, which can quickly and conveniently mock functions and variables for testing.

* Mockey is not intrusive to your code, you do not need to change your current code for testing.
* Mockey is easy to use, no matter how your code is written.
* Mockey has adaption to various of platforms, and more in future.

 Mockey is widely used in the unit test of ByteDance services, CloudWeGo etc.

<p align="right">(<a href="#readme-top">back to top</a>)</p>


<a name="built-with"> </a>
### Built With

* linux / macOS / Windows
* arm64 / amd64
* go1.13 +

<p align="right">(<a href="#readme-top">back to top</a>)</p>


<a name="quick-start"></a>

## Quick Start

<a name="installation"></a>

## Installation
`go get github.com/bytedance/mockey`

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<a name="demo"></a>

### Demo
Here is a demo of how to use mockey
1. Install mockey `go get github.com/bytedance/mockey`
2. Add a test case:
```go
func TestXxx(t *testing.T) {
	name := func() string { return "Alice" }
	mockey.Mock(name).Return("Bob").Build()
	fmt.Printf("I am %s", name())
}
```
3. run `go test -gcflags="all=-l -N" ./... -v`  
you can find that The out put is `I am Bob`, not `I am Alice`

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<a href="examples"></a>

## Examples

Here are examples of how to use mockey to mock `simple function`, `struct function` and `value`:

<a name="mock-func"> </a>

### Mock simple function
```go
func TestMockFunc(t *testing.T) {
	sum := func(i, j int) int { return i + j }
	mockey.PatchConvey("mock function, sum for example", t, func() {
		mocker := mockey.Mock(sum).Return(888).Build()
		sum(1, 2) // here sum will return 888

		var origin func(i, j int) int
		mocker.Origin(&origin) // now origin is set to the original sum
		mocker.To(func(i, j int) int {
			fmt.Println("sum is mocked")
			return origin(i, j)
		}) // re-mock sum
		sum(1, 2) // here sum will return 3, and the word "sum is mocked" will be printed

		// When leaving 'PatchConvey', all mocks in convey will be unpatched
		// You can also un-patch them by calling mocker.UnPatch()
	})
	sum(1, 2) // original sum function call with return value 3
}
```
<p align="right">(<a href="#readme-top">back to top</a>)</p>

<a name="mock-struct"> </a>

### Mock struct function
```go
```
<p align="right">(<a href="#readme-top">back to top</a>)</p>

<a name="mock-value"> </a>

### Mock value
```go
```
<p align="right">(<a href="#readme-top">back to top</a>)</p>

## Usage
here is api usage documentary
<p align="right">(<a href="#readme-top">back to top</a>)</p>



<!-- ROADMAP -->
## Roadmap
<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- CONTRIBUTING -->
## Contributing
<p align="right">(<a href="#readme-top">back to top</a>)</p>

## License

Distributed under the  Apache License, Version 2.0. See [LICENSE](./LICENSE-APACHE) for more information.

<p align="right">(<a href="#readme-top">back to top</a>)</p>




